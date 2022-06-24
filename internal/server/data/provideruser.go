package data

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/infrahq/infra/internal"
	"github.com/infrahq/infra/internal/server/models"
	"github.com/infrahq/infra/uid"
)

func CreateProviderUser(db *gorm.DB, provider *models.Provider, ident *models.Identity) (*models.ProviderUser, error) {
	pu, err := get[models.ProviderUser](db, ByIdentityID(ident.ID), ByProviderID(provider.ID))
	if err != nil && !errors.Is(err, internal.ErrNotFound) {
		return nil, err
	}

	if pu == nil {
		pu = &models.ProviderUser{
			ProviderID: provider.ID,
			IdentityID: ident.ID,
			Email:      ident.Name,
			LastUpdate: time.Now().UTC(),
		}
		if err := add(db, pu); err != nil {
			return nil, err
		}
	}

	// If there were other attributes to update, I guess they should be updated here.

	return pu, nil
}

func UpdateProviderUser(db *gorm.DB, providerUser *models.ProviderUser) error {
	return save(db, providerUser)
}

func ListProviderUsers(db *gorm.DB, p *models.Pagination, selectors ...SelectorFunc) ([]models.ProviderUser, error) {
	return list[models.ProviderUser](db, p, selectors...)
}

func DeleteProviderUsers(db *gorm.DB, selectors ...SelectorFunc) error {
	return deleteAll[models.ProviderUser](db, selectors...)
}

func GetProviderUser(db *gorm.DB, providerID, userID uid.ID) (*models.ProviderUser, error) {
	return get[models.ProviderUser](db, ByProviderID(providerID), ByIdentityID(userID))
}
