package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/mpraski/api-gateway/store"
	"github.com/mpraski/api-gateway/token"
)

type (
	TokenReference struct {
		setter          store.Setter
		valueParser     token.Parser
		referenceParser token.Parser
		referenceIssuer token.Issuer
	}

	TokenReferenceServer struct {
		reference *TokenReference
	}

	referenceRequest struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
)

const (
	accessExpiration  = time.Hour
	refreshExpiration = 12 * time.Hour
)

func (t *TokenReference) Make(ctx context.Context, value string, expiration time.Duration) (token.Token, error) {
	v, err := t.valueParser.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("failed to parse value token: %w", err)
	}

	r, err := t.referenceIssuer.Issue()
	if err != nil {
		return nil, fmt.Errorf("failed to issue reference token: %w", err)
	}

	if err := t.setter.Set(ctx, r.String(), v.String(), expiration); err != nil {
		return nil, fmt.Errorf("failed to associate tokens: %w", err)
	}

	return r, nil
}

func (t *TokenReference) Delete(ctx context.Context, reference string) error {
	r, err := t.referenceParser.Parse(reference)
	if err != nil {
		return fmt.Errorf("failed to parse reference token: %w", err)
	}

	if err := t.setter.Del(ctx, r.String()); err != nil {
		return fmt.Errorf("failed to delete token association: %w", err)
	}

	return nil
}

func NewTokenReferenceServer(reference *TokenReference) *TokenReferenceServer {
	return &TokenReferenceServer{reference: reference}
}

func (s *TokenReferenceServer) HandleAssociation(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.CreateAssociation(w, r)
	case http.MethodDelete:
		s.DeleteAssociation(w, r)
	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

func (s *TokenReferenceServer) CreateAssociation(w http.ResponseWriter, r *http.Request) {
	var request referenceRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	if request.AccessToken == "" || request.RefreshToken == "" {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	newAccess, err := s.reference.Make(r.Context(), request.AccessToken, accessExpiration)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusPreconditionFailed), http.StatusPreconditionFailed)
		return
	}

	newRefresh, err := s.reference.Make(r.Context(), request.RefreshToken, refreshExpiration)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusPreconditionFailed), http.StatusPreconditionFailed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	_ = json.NewEncoder(w).Encode(referenceRequest{
		AccessToken:  newAccess.String(),
		RefreshToken: newRefresh.String(),
	})
}

func (s *TokenReferenceServer) DeleteAssociation(w http.ResponseWriter, r *http.Request) {
	var request referenceRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	if request.AccessToken == "" || request.RefreshToken == "" {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	if err := s.reference.Delete(r.Context(), request.AccessToken); err != nil {
		http.Error(w, http.StatusText(http.StatusPreconditionFailed), http.StatusPreconditionFailed)
		return
	}

	if err := s.reference.Delete(r.Context(), request.RefreshToken); err != nil {
		http.Error(w, http.StatusText(http.StatusPreconditionFailed), http.StatusPreconditionFailed)
		return
	}

	w.WriteHeader(http.StatusOK)
}
