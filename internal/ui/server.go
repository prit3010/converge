package ui

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"

	"github.com/prittamravi/converge/internal/core"
	"github.com/prittamravi/converge/internal/llm"
)

type Server struct {
	svc      *core.Service
	comparer *llm.Comparer
	tmpl     *template.Template
	mux      *http.ServeMux
}

func NewServer(svc *core.Service) (*Server, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/*.html", "templates/partials/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	s := &Server{
		svc:      svc,
		comparer: llm.NewComparer(svc.DB, svc.Store),
		tmpl:     tmpl,
		mux:      http.NewServeMux(),
	}
	s.routes()
	return s, nil
}

func (s *Server) routes() {
	staticSub, err := fs.Sub(staticFS, "static")
	if err == nil {
		s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))
		s.mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
			f, openErr := staticSub.Open("favicon.svg")
			if openErr != nil {
				http.NotFound(w, r)
				return
			}
			defer f.Close()

			data, readErr := io.ReadAll(f)
			if readErr != nil {
				http.Error(w, "failed to load favicon", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "image/svg+xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
		})
	}
	s.mux.HandleFunc("GET /", s.handleIndex)
	s.mux.HandleFunc("GET /api/cells", s.handleAPICells)
	s.mux.HandleFunc("GET /api/cell/{id}", s.handleAPICell)
	s.mux.HandleFunc("GET /api/diff/{cellA}/{cellB}", s.handleAPIDiff)
	s.mux.HandleFunc("GET /api/branches", s.handleAPIBranches)
	s.mux.HandleFunc("GET /api/ui/summary", s.handleAPIUISummary)
	s.mux.HandleFunc("POST /api/compare", s.handleAPICompare)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) ListenAndServe(addr string) error {
	fmt.Printf("Converge UI running at http://%s\n", addr)
	return http.ListenAndServe(addr, s)
}
