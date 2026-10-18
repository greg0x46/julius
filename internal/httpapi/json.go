package httpapi

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"
)

// maxJSONBody caps non-multipart request bodies; handlers may set a tighter
// limit of their own. Statement uploads are multipart and keep their own cap.
const maxJSONBody = 1 << 20

// limitBody rejects oversized JSON bodies up front (413) when the length is
// declared, and caps undeclared (chunked) ones so decoding fails instead of
// buffering without bound.
func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && r.Body != http.NoBody && !isMultipart(r) {
			if r.ContentLength > maxJSONBody {
				writeProblem(w, http.StatusRequestEntityTooLarge, "body-too-large", "Solicitação muito grande", "O limite é 1 MB.")
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
		}
		next.ServeHTTP(w, r)
	})
}

func isMultipart(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && strings.HasPrefix(mediaType, "multipart/")
}

// decodeStrict mirrors the reference API's StrictModel (extra="forbid"):
// unknown fields in the request body are rejected rather than silently
// ignored, and trailing data after the JSON value is an error too.
func decodeStrict(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != nil {
		if err == io.EOF {
			return nil
		}
		return err
	}
	return errTrailingData
}

var errTrailingData = jsonTrailingDataError{}

type jsonTrailingDataError struct{}

func (jsonTrailingDataError) Error() string { return "unexpected trailing data after JSON body" }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
