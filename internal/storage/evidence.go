package storage

import (
	"crypto/sha256"
	"database/sql"
	"fmt"

	"github.com/CJhuochai/project-brain/internal/extract"
)

// SymbolID excludes line numbers so inserting a comment does not change identity.
func SymbolID(repository, path, name, signature string) string {
	return fmt.Sprintf("sym:%x", sha256.Sum256([]byte(repository+"\x00"+path+"\x00"+name+"\x00"+signature)))
}

func (db *DB) ReplaceEvidenceTx(transaction *sql.Tx, repositoryID, path string, result extract.Result) error {
	if _, err := transaction.Exec(`DELETE FROM contracts WHERE repository_id = ? AND path = ?`, repositoryID, path); err != nil {
		return err
	}
	if _, err := transaction.Exec(`DELETE FROM symbols WHERE repository_id = ? AND path = ?`, repositoryID, path); err != nil {
		return err
	}
	if _, err := transaction.Exec(`DELETE FROM edges WHERE repository_id = ? AND path = ?`, repositoryID, path); err != nil {
		return err
	}
	if _, err := transaction.Exec(`DELETE FROM diagnostics WHERE repository_id = ? AND path = ?`, repositoryID, path); err != nil {
		return err
	}
	for _, symbol := range result.Symbols {
		if _, err := transaction.Exec(`INSERT INTO symbols(repository_id, path, name, kind, line, uid, signature, end_line) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, repositoryID, path, symbol.Name, symbol.Kind, symbol.Line, SymbolID(repositoryID, path, symbol.Name, symbol.Signature), symbol.Signature, symbol.EndLine); err != nil {
			return err
		}
	}
	for _, edge := range result.Edges {
		if _, err := transaction.Exec(`INSERT INTO edges(repository_id, path, source, target, kind, line, confidence, source_signature, target_arity) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`, repositoryID, path, edge.Source, edge.Target, edge.Kind, edge.Line, edge.Confidence, edge.SourceSignature, edge.TargetArity); err != nil {
			return err
		}
	}
	for _, contract := range result.Contracts {
		if _, err := transaction.Exec(`INSERT INTO contracts(repository_id,path,kind,role,service,contract_key,symbol,signature,line,confidence,reason,broker) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, repositoryID, path, contract.Kind, contract.Role, contract.Service, contract.Key, contract.Symbol, contract.Signature, contract.Line, contract.Confidence, contract.Reason, contract.Broker); err != nil {
			return err
		}
	}
	for _, diagnostic := range result.Diagnostics {
		if _, err := transaction.Exec(`INSERT INTO diagnostics(repository_id, path, message, line, confidence) VALUES(?, ?, ?, ?, ?)`, repositoryID, path, diagnostic.Message, diagnostic.Line, diagnostic.Confidence); err != nil {
			return err
		}
	}
	return nil
}
