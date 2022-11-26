package data

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/infrahq/infra/internal"
	"github.com/infrahq/infra/uid"
)

func TestCreateSettings(t *testing.T) {
	runDBTests(t, func(t *testing.T, db *DB) {
		err := createSettings(db, 145)
		assert.NilError(t, err)

		settings, err := getSettingsForOrg(db, 145)
		assert.NilError(t, err)
		assert.Assert(t, settings.ID != 0)
		assert.Assert(t, len(settings.PrivateJWK) != 0)
		assert.Assert(t, len(settings.PublicJWK) != 0)
		assert.Equal(t, settings.LengthMin, 8)
	})
}
func TestGetSettings(t *testing.T) {
	runDBTests(t, func(t *testing.T, db *DB) {
		t.Run("success", func(t *testing.T) {
			tx := txnForTestCase(t, db, 181)

			err := createSettings(db, 181)
			assert.NilError(t, err)

			settings, err := GetSettings(tx)
			assert.NilError(t, err)
			assert.Equal(t, settings.OrganizationID, uid.ID(181))
		})
		t.Run("not found", func(t *testing.T) {
			tx := txnForTestCase(t, db, 77)
			_, err := GetSettings(tx)
			assert.ErrorIs(t, err, internal.ErrNotFound)
		})
	})
}

func TestUpdateSettings(t *testing.T) {
	runDBTests(t, func(t *testing.T, db *DB) {
		tx := txnForTestCase(t, db, 181)

		err := createSettings(db, 181)
		assert.NilError(t, err)
		orig, err := GetSettings(tx)
		assert.NilError(t, err)

		updated := *orig // shallow copy
		updated.PrivateJWK = "primary-key"
		updated.LengthMin = 2
		updated.SymbolMin = 3

		err = UpdateSettings(tx, &updated)
		assert.NilError(t, err)

		actual, err := GetSettings(tx)
		assert.NilError(t, err)

		assert.Assert(t, orig.UpdatedAt != actual.UpdatedAt)
		assert.DeepEqual(t, actual, &updated, cmpModel)
	})
}
