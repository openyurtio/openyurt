package server

import (
	"fmt"
	"net/http"

	"github.com/alibaba/openyurt/cmd/yurthub/app/config"
	"github.com/alibaba/openyurt/pkg/yurthub/certificate/interfaces"
	"github.com/alibaba/openyurt/pkg/yurthub/profile"
	"github.com/gorilla/mux"
)

// Server is an interface for providing http service for yurthub
type Server interface {
	Run()
}

type yurtHubServer struct {
	mux            *mux.Router
	certificateMgr interfaces.YurtCertificateManager
	proxyHandler   http.Handler
	cfg            *config.YurtHubConfiguration
}

// NewYurtHubServer creates a Server object
func NewYurtHubServer(cfg *config.YurtHubConfiguration,
	certificateMgr interfaces.YurtCertificateManager,
	proxyHandler http.Handler) Server {
	return &yurtHubServer{
		mux:            mux.NewRouter(),
		certificateMgr: certificateMgr,
		proxyHandler:   proxyHandler,
		cfg:            cfg,
	}
}

func (s *yurtHubServer) Run() {
	s.registerHandler()

	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", s.cfg.YurtHubHost, s.cfg.YurtHubPort),
		Handler: s.mux,
	}

	err := server.ListenAndServe()
	if err != nil {
		panic(err)
	}
}

func (s *yurtHubServer) registerHandler() {
	// register handler for health check
	s.mux.HandleFunc("/v1/healthz", s.healthz).Methods("GET")

	// register handler for profile
	profile.Install(s.mux)

	// attention: "/" route must be put at the end of registerHandler
	// register handlers for proxy to kube-apiserver
	s.mux.PathPrefix("/").Handler(s.proxyHandler)
}

func (s *yurtHubServer) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK")
}
