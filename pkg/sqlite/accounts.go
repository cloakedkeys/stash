package sqlite

import (
	"fmt"

	"github.com/doug-martin/goqu/v9"
)

// accountTables hold the accounts, parents first. A metadata export doesn't
// include them, so they have to be carried across a reset.
var accountTables = []string{"users", "user_roles", "user_ui_config"}

func (db *Database) readAccounts() (map[string][]goqu.Record, error) {
	ret := make(map[string][]goqu.Record)
	for _, table := range accountTables {
		rows, err := db.writeDB.Queryx("SELECT * FROM " + table)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", table, err)
		}

		for rows.Next() {
			row := make(map[string]interface{})
			if err := rows.MapScan(row); err != nil {
				rows.Close()
				return nil, fmt.Errorf("reading %s: %w", table, err)
			}
			ret[table] = append(ret[table], goqu.Record(row))
		}

		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", table, err)
		}
	}

	return ret, nil
}

// restoreAccounts replaces the default account made by the migrations.
func (db *Database) restoreAccounts(accounts map[string][]goqu.Record) error {
	if len(accounts["users"]) == 0 {
		return nil
	}

	tx, err := db.writeDB.Beginx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("DELETE FROM users"); err != nil {
		return err
	}

	for _, table := range accountTables {
		for _, row := range accounts[table] {
			query, args, err := dialect.Insert(table).Prepared(true).Rows(row).ToSQL()
			if err != nil {
				return fmt.Errorf("restoring %s: %w", table, err)
			}
			if _, err := tx.Exec(query, args...); err != nil {
				return fmt.Errorf("restoring %s: %w", table, err)
			}
		}
	}

	return tx.Commit()
}
