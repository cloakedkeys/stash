package postgres

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
		records, err := db.readAccountTable(table)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", table, err)
		}
		ret[table] = records
	}

	return ret, nil
}

func (db *Database) readAccountTable(table string) ([]goqu.Record, error) {
	rows, err := db.writeDB.Queryx("SELECT * FROM " + table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ret []goqu.Record
	for rows.Next() {
		row := make(map[string]interface{})
		if err := rows.MapScan(row); err != nil {
			return nil, err
		}
		ret = append(ret, goqu.Record(row))
	}

	return ret, rows.Err()
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

	if _, err := tx.Exec("SELECT setval(pg_get_serial_sequence('users','id'), (SELECT MAX(id) FROM users))"); err != nil {
		return err
	}

	return tx.Commit()
}
