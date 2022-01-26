package service

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/mpraski/api-gateway/authentication"
)

type (
	TokenReferenceServer struct {
		reference *authentication.TokenReference
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

func NewTokenReferenceServer(reference *authentication.TokenReference) *TokenReferenceServer {
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

	if request.AccessToken == "" ||
		request.RefreshToken == "" ||
		request.AccessToken == request.RefreshToken {
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

	if request.AccessToken == "" ||
		request.RefreshToken == "" ||
		request.AccessToken == request.RefreshToken {
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
