// Package api serves the f1mcp tools as a JSON web API, for the chat front
// end: the same tools as over MCP, with their images as links.
//
//	GET  /api/tools         the tools, with descriptions and input schemas
//	POST /api/tools/{name}  call a tool with JSON arguments
//	GET  /img/{id}          an image a tool returned
package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/mariuspot/f1mcp/internal/tools"
)

// maxBody is the largest request body accepted, in bytes.
const maxBody = 64 << 10

// Result is a tool's answer: its data, and links to its images.
type Result struct {
	Result any         `json:"result"`
	Images []ImageLink `json:"images,omitempty"`
}

// ImageLink is where to get an image a tool returned.
type ImageLink struct {
	Name     string `json:"name"` // what it shows, e.g. "map", "faster"
	URL      string `json:"url"`  // e.g. "/img/3f2a….png"
	MIMEType string `json:"mime_type"`
}

type errorBody struct {
	Error string `json:"error"`
}

// Handler serves the API. Images are kept in memory, up to maxImageBytes in
// all, the oldest dropped first.
func Handler(r *tools.Registry, maxImageBytes int) http.Handler {
	h := &handler{tools: r, images: newImageStore(maxImageBytes)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/tools", h.list)
	mux.HandleFunc("POST /api/tools/{name}", h.call)
	mux.HandleFunc("GET /img/{id}", h.image)
	return mux
}

type handler struct {
	tools  *tools.Registry
	images *imageStore
}

func (h *handler) list(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.tools.Tools())
}

func (h *handler) call(w http.ResponseWriter, req *http.Request) {
	t := h.tools.Tool(req.PathValue("name"))
	if t == nil {
		writeJSON(w, http.StatusNotFound, errorBody{"no tool " + req.PathValue("name")})
		return
	}
	args, err := io.ReadAll(http.MaxBytesReader(w, req.Body, maxBody))
	if err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, errorBody{err.Error()})
		return
	}
	out, imgs, err := t.Call(req.Context(), args)
	switch {
	case errors.Is(err, tools.ErrBadArguments):
		writeJSON(w, http.StatusBadRequest, errorBody{err.Error()})
		return
	case req.Context().Err() != nil:
		return // the client has gone
	case err != nil:
		// The tool's own errors, e.g. "no red flag in …", are answers the
		// caller can act on.
		writeJSON(w, http.StatusUnprocessableEntity, errorBody{err.Error()})
		return
	}
	res := Result{Result: out}
	for _, img := range imgs {
		res.Images = append(res.Images, ImageLink{Name: img.Name, URL: "/img/" + h.images.put(img.Data, img.MIMEType), MIMEType: img.MIMEType})
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *handler) image(w http.ResponseWriter, req *http.Request) {
	img, ok := h.images.get(req.PathValue("id"))
	if !ok {
		http.NotFound(w, req)
		return
	}
	w.Header().Set("Content-Type", img.mimeType)
	// An image's ID is its content's hash, so it never changes.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Write(img.data)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: writing response: %v", err)
	}
}

// imageStore keeps images by the hash of their content.
type imageStore struct {
	mu    sync.Mutex
	max   int
	size  int
	byID  map[string]storedImage
	order []string // oldest first
}

type storedImage struct {
	data     []byte
	mimeType string
}

var extensions = map[string]string{"image/png": ".png", "image/gif": ".gif"}

func newImageStore(max int) *imageStore {
	return &imageStore{max: max, byID: map[string]storedImage{}}
}

// put stores an image and returns its ID, e.g. "3f2a….png".
func (s *imageStore) put(data []byte, mimeType string) string {
	sum := sha256.Sum256(data)
	id := hex.EncodeToString(sum[:12]) + extensions[mimeType]
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[id]; ok {
		return id
	}
	s.byID[id] = storedImage{data, mimeType}
	s.order = append(s.order, id)
	s.size += len(data)
	for s.size > s.max && len(s.order) > 1 {
		old := s.order[0]
		s.order = s.order[1:]
		s.size -= len(s.byID[old].data)
		delete(s.byID, old)
	}
	return id
}

func (s *imageStore) get(id string) (storedImage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	img, ok := s.byID[strings.TrimSpace(id)]
	return img, ok
}
