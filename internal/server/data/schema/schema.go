package schema

import (
	"bufio"
	"strings"
)

type Table interface {
	// Table returns the name of the database table used to store this type.
	Table() string
	// Schema returns CREATE TABLE sql statement used to create the database
	// table.
	Schema() string
}

type Column struct {
	// Name of the column
	Name string
	// DataType is the sql data type of the column.
	DataType string
}

type TableDescription struct {
	Name    string
	Columns []Column
}

// ParseSchema parses an SQL CREATE TABLE statement and returns the table name
// and all column names and types. The schema string is expected to start with
// the following structure. Any [...] can be replaced by any words.
//
//    CREATE TABLE [...] TableName (
//        Column.Name Column.DataType [...] ,
//        <more columns>
//    ) [...]`
//
func ParseSchema(table Table) (TableDescription, error) {
	var cols []Column
	var state uint8
	var name string
	words := bufio.NewScanner(strings.NewReader(table.Schema()))
	words.Split(bufio.ScanWords)

SCAN:
	for words.Scan() {
		word := words.Text()
		switch state {
		case scanForStartOFColumns:
			if word == "(" {
				state = scanForColumnName
				continue
			}
			name = strings.TrimSuffix(word, "(") // store the last word before "("

		case scanForColumnName:
			cols = append(cols, Column{Name: word})
			state = scanForColumnDataType

		case scanForColumnDataType:
			cols[len(cols)-1].DataType = strings.TrimSuffix(word, ",")

			if strings.HasSuffix(word, ",") { // new column
				state = scanForColumnName
				continue
			}
			state = scanForEndOfColumnDefinition

		case scanForEndOfColumnDefinition: // , or )
			if strings.HasSuffix(word, ",") { // new column
				state = scanForColumnName
				continue
			}
			if strings.HasSuffix(word, ")") || strings.HasSuffix(word, ");") {
				break SCAN // end of possible columns
			}
		}
	}

	return TableDescription{Name: name, Columns: cols}, words.Err()
}

const (
	scanForStartOFColumns = iota
	scanForColumnName
	scanForColumnDataType
	scanForEndOfColumnDefinition
)
