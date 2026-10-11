package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type supplierAccountsHandlerRepo struct {
	service.UpstreamCenterRepository
	query service.UpstreamSupplierAccountsQuery
}

func (r *supplierAccountsHandlerRepo) GetSupplier(_ context.Context, id int64) (*service.UpstreamSupplier, error) {
	if id != 5 {
		return nil, service.ErrUpstreamNotFound
	}
	return &service.UpstreamSupplier{ID: 5, Website: "https://site.example"}, nil
}

func (r *supplierAccountsHandlerRepo) ListSupplierAccounts(_ context.Context, query service.UpstreamSupplierAccountsQuery) ([]*service.UpstreamSupplierAccount, int64, error) {
	r.query = query
	return []*service.UpstreamSupplierAccount{{ID: 8, Name: "Site Key", Platform: "openai", Endpoint: "https://site.example/v1"}}, 1, nil
}

func TestUpstreamSupplierAccountsHandlerContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &supplierAccountsHandlerRepo{}
	h := NewUpstreamCenterHandler(service.NewUpstreamCenterService(repo, nil, nil, nil), nil)
	router := gin.New()
	router.GET("/suppliers/:id/accounts", h.SupplierAccounts)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/suppliers/5/accounts?page=2&page_size=25&search=Site", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.Equal(t, service.UpstreamSupplierAccountsQuery{Origins: []string{"https://site.example"}, Page: 2, PageSize: 25, Search: "Site"}, repo.query)
	var envelope struct {
		Data service.UpstreamSupplierAccountsPage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data.Items, 1)
	require.Equal(t, int64(8), envelope.Data.Items[0].ID)
	require.NotContains(t, w.Body.String(), "credentials")
	require.NotContains(t, w.Body.String(), "api_key")
	for _, tc := range []struct {
		id     string
		status int
	}{{"0", http.StatusBadRequest}, {"invalid", http.StatusBadRequest}, {"6", http.StatusNotFound}} {
		w = httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/suppliers/"+tc.id+"/accounts", nil))
		require.Equal(t, tc.status, w.Code)
	}
}
