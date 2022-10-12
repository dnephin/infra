package access

import (
	"github.com/gin-gonic/gin"

	"github.com/infrahq/infra/internal"
	"github.com/infrahq/infra/internal/server/data"
	"github.com/infrahq/infra/internal/server/models"
)

func ForgotDomainRequest(c *gin.Context, email string) ([]models.ForgottenDomain, error) {
	// no auth required
	rCtx := GetRequestContext(c)

	domains, err := data.GetForgottenDomainsForEmail(rCtx.DBTxn, email)
	if err != nil {
		return nil, err
	}

	if len(domains) < 1 {
		return nil, internal.ErrNotFound
	}

	return domains, nil
}
