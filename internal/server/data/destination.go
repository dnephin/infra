package data

import (
	"fmt"
	"time"

	"github.com/infrahq/infra/internal/server/data/querybuilder"
	"github.com/infrahq/infra/internal/server/models"
	"github.com/infrahq/infra/uid"
)

type destinationsTable models.Destination

func (d destinationsTable) Table() string {
	return "destinations"
}

func (d destinationsTable) Columns() []string {
	return []string{"connection_ca", "connection_url", "created_at", "deleted_at", "id", "kind", "last_seen_at", "name", "organization_id", "resources", "roles", "unique_id", "updated_at", "version"}
}

func (d destinationsTable) Values() []any {
	return []any{d.ConnectionCA, d.ConnectionURL, d.CreatedAt, d.DeletedAt, d.ID, d.Kind, d.LastSeenAt, d.Name, d.OrganizationID, d.Resources, d.Roles, d.UniqueID, d.UpdatedAt, d.Version}
}

func (d *destinationsTable) ScanFields() []any {
	return []any{&d.ConnectionCA, &d.ConnectionURL, &d.CreatedAt, &d.DeletedAt, &d.ID, &d.Kind, &d.LastSeenAt, &d.Name, &d.OrganizationID, &d.Resources, &d.Roles, &d.UniqueID, &d.UpdatedAt, &d.Version}
}

func validateDestination(dest *models.Destination) error {
	if dest.Name == "" {
		return fmt.Errorf("Destination.Name is required")
	}
	if dest.Kind == "" {
		return fmt.Errorf("Destination.Kind is required")
	}
	return nil
}

func CreateDestination(tx WriteTxn, destination *models.Destination) error {
	if err := validateDestination(destination); err != nil {
		return err
	}
	return insert(tx, (*destinationsTable)(destination))
}

func UpdateDestination(tx WriteTxn, destination *models.Destination) error {
	if err := validateDestination(destination); err != nil {
		return err
	}
	return update(tx, (*destinationsTable)(destination))
}

type GetDestinationOptions struct {
	// ByID instructs GetDestination to return the row matching this ID. When
	// this value is set, all other fields on this strut will be ignored
	ByID uid.ID
	// ByUniqueID instructs GetDestination to return the row matching this
	// uniqueID.
	ByUniqueID string
	// ByName instructs GetDestination to return the row matching this name.
	ByName string
}

func GetDestination(tx ReadTxn, opts GetDestinationOptions) (*models.Destination, error) {
	destination := destinationsTable{}
	query := querybuilder.New("SELECT")
	query.B(columnsForSelect(destination))
	query.B("FROM destinations")
	query.B("WHERE deleted_at is null AND organization_id = ?", tx.OrganizationID())

	switch {
	case opts.ByID != 0:
		query.B("AND id = ?", opts.ByID)
	case opts.ByUniqueID != "":
		query.B("AND unique_id = ?", opts.ByUniqueID)
	case opts.ByName != "":
		query.B("AND name = ?", opts.ByName)
	default:
		return nil, fmt.Errorf("an ID is required to GetDestination")
	}

	err := tx.QueryRow(query.String(), query.Args...).Scan(destination.ScanFields()...)
	if err != nil {
		return nil, handleError(err)
	}
	return (*models.Destination)(&destination), nil
}

type ListDestinationsOptions struct {
	ByUniqueID string
	ByName     string
	ByKind     string

	Pagination *Pagination
}

func ListDestinations(tx ReadTxn, opts ListDestinationsOptions) ([]models.Destination, error) {
	table := destinationsTable{}
	query := querybuilder.New("SELECT")
	query.B(columnsForSelect(table))
	if opts.Pagination != nil {
		query.B(", count(*) OVER()")
	}
	query.B("FROM destinations")
	query.B("WHERE deleted_at is null")
	query.B("AND organization_id = ?", tx.OrganizationID())

	if opts.ByUniqueID != "" {
		query.B("AND unique_id = ?", opts.ByUniqueID)
	}
	if opts.ByName != "" {
		query.B("AND name = ?", opts.ByName)
	}
	if opts.ByKind != "" {
		query.B("AND kind = ?", opts.ByKind)
	}

	query.B("ORDER BY name")
	if opts.Pagination != nil {
		opts.Pagination.PaginateQuery(query)
	}

	rows, err := tx.Query(query.String(), query.Args...)
	if err != nil {
		return nil, err
	}
	return scanRows(rows, func(d *models.Destination) []any {
		fields := (*destinationsTable)(d).ScanFields()
		if opts.Pagination != nil {
			fields = append(fields, &opts.Pagination.TotalCount)
		}
		return fields
	})
}

func DeleteDestination(tx WriteTxn, id uid.ID) error {
	stmt := `
		UPDATE destinations SET deleted_at = ?
		WHERE id = ? AND organization_id = ? AND deleted_at is null
	`
	_, err := tx.Exec(stmt, time.Now(), id, tx.OrganizationID())
	return handleError(err)
}

type DestinationsCount struct {
	Connected bool
	Version   string
	Count     float64
}

func CountDestinationsByConnectedVersion(tx ReadTxn) ([]DestinationsCount, error) {
	timeout := time.Now().Add(-5 * time.Minute)

	stmt := `
		SELECT COALESCE(version, '') as version, last_seen_at >= ? as connected, count(*)
		FROM destinations
		WHERE deleted_at IS NULL
		GROUP BY connected, version
	`
	rows, err := tx.Query(stmt, timeout)
	if err != nil {
		return nil, err
	}
	return scanRows(rows, func(item *DestinationsCount) []any {
		return []any{&item.Version, &item.Connected, &item.Count}
	})
}

func CountAllDestinations(tx ReadTxn) (int64, error) {
	return countRows(tx, destinationsTable{})
}
