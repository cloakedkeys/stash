package sqlite

import "github.com/doug-martin/goqu/v9"

var (
	storiesFilesJoinTable = goqu.T(storiesFilesTable)
	storiesTagsJoinTable  = goqu.T(storiesTagsTable)
	storiesURLsJoinTable  = goqu.T(storiesURLsTable)
	storyTableMgr         = &table{
		table:    goqu.T(storyTable),
		idColumn: goqu.T(storyTable).Col(idColumn),
	}
	storiesFilesTableMgr = &relatedFilesTable{
		table: table{
			table:    storiesFilesJoinTable,
			idColumn: storiesFilesJoinTable.Col(storyIDColumn),
		},
	}
	storiesTagsTableMgr = &joinTable{
		table: table{
			table:    storiesTagsJoinTable,
			idColumn: storiesTagsJoinTable.Col(storyIDColumn),
		},
		fkColumn:     storiesTagsJoinTable.Col(tagIDColumn),
		foreignTable: tagTableMgr,
		orderBy:      tagTableSort,
	}
	storiesURLsTableMgr = &orderedValueTable[string]{
		table: table{
			table:    storiesURLsJoinTable,
			idColumn: storiesURLsJoinTable.Col(storyIDColumn),
		},
		valueColumn: storiesURLsJoinTable.Col(storyURLColumn),
	}
)
