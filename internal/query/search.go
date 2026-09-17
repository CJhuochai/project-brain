package query

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/CJhuochai/project-brain/internal/storage"
)

const defaultSearchLimit = 1000
const searchCandidateLimit = 1000

type Evidence struct {
	ID               string   `json:"id,omitempty"`
	Signature        string   `json:"signature,omitempty"`
	EndLine          int      `json:"end_line,omitempty"`
	SourceID         string   `json:"source_id,omitempty"`
	TargetID         string   `json:"target_id,omitempty"`
	TargetRepository string   `json:"target_repository,omitempty"`
	Score            float64  `json:"score,omitempty"`
	MatchSources     []string `json:"match_sources,omitempty"`
	Reason           string   `json:"reason,omitempty"`
	Repository       string   `json:"repository"`
	File             string   `json:"file"`
	Line             int      `json:"line"`
	Name             string   `json:"name"`
	Kind             string   `json:"kind"`
	Confidence       string   `json:"confidence"`
	Source           string   `json:"source,omitempty"`
	Target           string   `json:"target,omitempty"`
}

type SearchResult struct {
	Evidence []Evidence `json:"evidence"`
	Coverage Coverage   `json:"coverage"`
}

// Search preserves the v1 slice contract. It is capped to avoid unbounded reads.
func Search(db *storage.DB, text string) ([]Evidence, error) {
	result, err := SearchRanked(db, text, defaultSearchLimit)
	return result.Evidence, err
}

// SearchRanked fuses bounded symbol, edge, FTS and literal substring candidates.
func SearchRanked(db *storage.DB, text string, limit int) (SearchResult, error) {
	return SearchRankedScoped(db, text, "", limit)
}

// SearchRankedScoped restricts every candidate source before its bounded query.
func SearchRankedScoped(db *storage.DB, text, repository string, limit int) (SearchResult, error) {
	result := SearchResult{Coverage: Coverage{Complete: true, Limit: limit}}
	text = strings.TrimSpace(text)
	if text == "" {
		return result, fmt.Errorf("search text is required")
	}
	if limit < 1 || limit > defaultSearchLimit {
		return result, fmt.Errorf("search limit must be between 1 and %d", defaultSearchLimit)
	}
	if repository != "" {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM files WHERE repository_id = ? UNION ALL SELECT 1 FROM symbols WHERE repository_id = ? UNION ALL SELECT 1 FROM edges WHERE repository_id = ? LIMIT 1)`, repository, repository, repository).Scan(&exists)
		if err != nil {
			return result, err
		}
		if !exists {
			return result, fmt.Errorf("repository scope not found: %s", repository)
		}
	}
	query, terms := normalizeIdentifier(text), identifierTerms(text)
	candidates := map[string]*Evidence{}
	add := func(item Evidence, score float64, source, reason string) {
		key := searchEvidenceKey(item)
		current, ok := candidates[key]
		if !ok {
			item.Score, item.Reason, item.MatchSources = score, reason, []string{source}
			candidates[key] = &item
			return
		}
		if score > current.Score {
			current.Score, current.Reason = score, reason
		}
		if !contains(current.MatchSources, source) {
			current.MatchSources = append(current.MatchSources, source)
		}
	}
	if truncated, err := searchSymbols(db, repository, text, terms, searchCandidateLimit, func(item Evidence) {
		add(item, symbolScore(item.Name, text, query), "symbol", symbolReason(item.Name, text, query))
	}); err != nil {
		return result, err
	} else if truncated {
		result.Coverage.Gap("symbol candidates truncated", true)
	}
	if truncated, err := searchEdges(db, repository, text, terms, searchCandidateLimit, func(item Evidence) {
		name := item.Target
		if edgeScore(item.Source, text, query) > edgeScore(name, text, query) {
			name = item.Source
		}
		add(item, edgeScore(name, text, query), "edge", edgeReason(name, text, query))
	}); err != nil {
		return result, err
	} else if truncated {
		result.Coverage.Gap("edge candidates truncated", true)
	}
	if fts := ftsQuery(terms); fts != "" {
		if truncated, err := searchFTSFiles(db, repository, fts, text, searchCandidateLimit, func(item Evidence, rank float64) { add(item, 300+ftsScore(rank), "file_fts", "full-text file match") }); err != nil {
			return result, err
		} else if truncated {
			result.Coverage.Gap("full-text file candidates truncated", true)
		}
	}
	pattern := "%" + escapeLike(strings.ToLower(text)) + "%"
	if truncated, err := searchSubstringFiles(db, repository, pattern, text, searchCandidateLimit, func(item Evidence) { add(item, 200, "file_substring", "literal file-content match") }); err != nil {
		return result, err
	} else if truncated {
		result.Coverage.Gap("substring file candidates truncated", true)
	}
	items := make([]Evidence, 0, len(candidates))
	for _, item := range candidates {
		sort.Strings(item.MatchSources)
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Score != items[j].Score {
			return items[i].Score > items[j].Score
		}
		if items[i].Repository != items[j].Repository {
			return items[i].Repository < items[j].Repository
		}
		if items[i].File != items[j].File {
			return items[i].File < items[j].File
		}
		if items[i].Line != items[j].Line {
			return items[i].Line < items[j].Line
		}
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		return items[i].Name < items[j].Name
	})
	result.Coverage.Matched = len(items)
	if len(items) > limit {
		items = items[:limit]
		result.Coverage.Gap("search result limit reached", true)
	}
	result.Evidence, result.Coverage.Returned = items, len(items)
	return result, nil
}

func searchSymbols(db *storage.DB, repository, exact string, terms []string, limit int, add func(Evidence)) (bool, error) {
	where, args := termsWhere("name", terms)
	where, args = scopeWhere(where, args, repository)
	args = append(args, strings.ToLower(exact), "%"+escapeLike(normalizeIdentifier(exact)), limit+1)
	return scanEvidence(db, `SELECT repository_id, path, line, name, kind, 'certain', uid, signature, end_line, '', '' FROM symbols WHERE `+where+` ORDER BY CASE WHEN lower(name) = ? THEN 0 WHEN replace(replace(replace(lower(name),'_',''),'-',''),'.','') LIKE ? ESCAPE '\' THEN 1 ELSE 2 END, repository_id, path, line, name LIMIT ?`, args, limit, add)
}
func searchEdges(db *storage.DB, repository, exact string, terms []string, limit int, add func(Evidence)) (bool, error) {
	where, args := termsWhere("source || ' ' || target", terms)
	where, args = scopeWhere(where, args, repository)
	return scanEvidence(db, `SELECT repository_id, path, line, target, kind, confidence, '', '', 0, source, target FROM edges WHERE `+where+` ORDER BY CASE WHEN lower(source)=? OR lower(target)=? THEN 0 ELSE 1 END, repository_id, path, line, target LIMIT ?`, append(args, strings.ToLower(exact), strings.ToLower(exact), limit+1), limit, add)
}
func searchFTSFiles(db *storage.DB, repository, fts, text string, limit int, add func(Evidence, float64)) (bool, error) {
	where, args := "file_fts MATCH ?", []any{fts}
	if repository != "" {
		where, args = where+" AND repository_id = ?", append(args, repository)
	}
	rows, err := db.Query(`SELECT repository_id, path, content, bm25(file_fts) FROM file_fts WHERE `+where+` ORDER BY bm25(file_fts), repository_id, path LIMIT ?`, append(args, limit+1)...)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var item Evidence
		var content string
		var rank float64
		if err := rows.Scan(&item.Repository, &item.File, &content, &rank); err != nil {
			return false, err
		}
		count++
		if count <= limit {
			item.Name, item.Kind, item.Confidence, item.Line = text, "text", "certain", matchLine(content, text)
			add(item, rank)
		}
	}
	return count > limit, rows.Err()
}
func searchSubstringFiles(db *storage.DB, repository, pattern, text string, limit int, add func(Evidence)) (bool, error) {
	where, args := scopeWhere("lower(content) LIKE ? ESCAPE '\\'", []any{pattern}, repository)
	rows, err := db.Query(`SELECT repository_id, path, content FROM files WHERE `+where+` ORDER BY repository_id, path LIMIT ?`, append(args, limit+1)...)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	return scanFiles(rows, text, limit, add)
}

func scanEvidence(db *storage.DB, statement string, args []any, limit int, add func(Evidence)) (bool, error) {
	rows, err := db.Query(statement, args...)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var item Evidence
		if err := rows.Scan(&item.Repository, &item.File, &item.Line, &item.Name, &item.Kind, &item.Confidence, &item.ID, &item.Signature, &item.EndLine, &item.Source, &item.Target); err != nil {
			return false, err
		}
		count++
		if count <= limit {
			add(item)
		}
	}
	return count > limit, rows.Err()
}
func scanFiles(rows *sql.Rows, text string, limit int, add func(Evidence)) (bool, error) {
	count := 0
	for rows.Next() {
		var item Evidence
		var content string
		if err := rows.Scan(&item.Repository, &item.File, &content); err != nil {
			return false, err
		}
		count++
		if count > limit {
			continue
		}
		item.Name, item.Kind, item.Confidence, item.Line = text, "text", "certain", matchLine(content, text)
		add(item)
	}
	return count > limit, rows.Err()
}

func termsWhere(column string, terms []string) (string, []any) {
	if len(terms) == 0 {
		return "1 = 0", nil
	}
	clauses, args := make([]string, 0, len(terms)), make([]any, 0, len(terms))
	for _, term := range terms {
		clauses, args = append(clauses, "lower("+column+") LIKE ? ESCAPE '\\'"), append(args, "%"+escapeLike(strings.ToLower(term))+"%")
	}
	return strings.Join(clauses, " AND "), args
}
func scopeWhere(where string, args []any, repository string) (string, []any) {
	if repository == "" {
		return where, args
	}
	return "repository_id = ? AND (" + where + ")", append([]any{repository}, args...)
}
func searchEvidenceKey(item Evidence) string {
	return strings.Join([]string{item.Repository, item.File, fmt.Sprint(item.Line), item.Name, item.Kind, item.Source, item.Target}, "\x00")
}
func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
func escapeLike(text string) string {
	return strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(text)
}
func matchLine(content, text string) int {
	if text == "" {
		return 1
	}
	index := strings.Index(strings.ToLower(content), strings.ToLower(text))
	if index < 0 {
		return 1
	}
	return strings.Count(content[:index], "\n") + 1
}
func ftsScore(rank float64) float64 {
	quality := -rank
	if quality < 0 {
		quality = 0
	}
	return quality / (60 + quality)
}

func normalizeIdentifier(text string) string {
	var builder strings.Builder
	for _, character := range text {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			builder.WriteRune(unicode.ToLower(character))
		}
	}
	return builder.String()
}
func identifierTerms(text string) []string {
	var terms []string
	var current []rune
	lastLower := false
	flush := func() {
		if len(current) > 0 {
			terms = append(terms, strings.ToLower(string(current)))
			current = nil
		}
		lastLower = false
	}
	for _, character := range text {
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) {
			flush()
			continue
		}
		if unicode.IsUpper(character) && lastLower {
			flush()
		}
		current = append(current, character)
		lastLower = unicode.IsLower(character)
	}
	flush()
	return terms
}
func ftsQuery(terms []string) string {
	quoted := make([]string, 0, len(terms))
	for _, term := range terms {
		if term != "" {
			quoted = append(quoted, `"`+strings.ReplaceAll(term, `"`, `""`)+`"`)
		}
	}
	return strings.Join(quoted, " AND ")
}
func identifierExact(name, query string) bool {
	if query == "" {
		return false
	}
	terms := identifierTerms(name)
	for start := range terms {
		for end := start + 1; end <= len(terms); end++ {
			if strings.Join(terms[start:end], "") == query {
				return true
			}
		}
	}
	return false
}
func symbolScore(name, text, query string) float64 {
	if strings.EqualFold(name, text) {
		return 1000
	}
	if identifierExact(name, query) {
		return 900
	}
	return 700
}
func symbolReason(name, text, query string) string {
	if strings.EqualFold(name, text) {
		return "exact symbol match"
	}
	if identifierExact(name, query) {
		return "exact normalized identifier match"
	}
	return "symbol identifier match"
}
func edgeScore(name, text, query string) float64 {
	if strings.EqualFold(name, text) {
		return 800
	}
	if identifierExact(name, query) {
		return 750
	}
	return 600
}
func edgeReason(name, text, query string) string {
	if strings.EqualFold(name, text) {
		return "exact edge endpoint match"
	}
	if identifierExact(name, query) {
		return "exact normalized edge endpoint match"
	}
	return "edge endpoint match"
}

func FileEvidence(db *storage.DB, repository, path string) ([]Evidence, error) {
	rows, err := db.Query(`SELECT repository_id, path, line, name, kind, 'certain', uid, signature, end_line, '', '' FROM symbols WHERE repository_id = ? AND path = ? UNION ALL SELECT repository_id, path, line, target, kind, confidence, '', '', 0, source, target FROM edges WHERE repository_id = ? AND path = ? ORDER BY line`, repository, path, repository, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []Evidence
	for rows.Next() {
		var item Evidence
		if err := rows.Scan(&item.Repository, &item.File, &item.Line, &item.Name, &item.Kind, &item.Confidence, &item.ID, &item.Signature, &item.EndLine, &item.Source, &item.Target); err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	return results, rows.Err()
}
