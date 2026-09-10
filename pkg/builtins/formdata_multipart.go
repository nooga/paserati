package builtins

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"strings"

	"github.com/nooga/paserati/pkg/vm"
)

// parseMultipartFormData parses a `multipart/form-data` body - per the
// Content-Type header's boundary parameter - into a real FormData instance
// (#397). Matches FormData's existing runtime representation
// (formdata_init.go): each part becomes a FormDataEntry, and a file-shaped
// part (one whose Content-Disposition carries a filename) becomes a real
// Blob (via NewBlobValue, added for #396) with its `.type` taken from the
// part's own Content-Type header, not the outer body's - so undici-style
// isBlobLike()/isFormDataLike() checks succeed on the round trip.
//
// Uses multipart.NewReader + NextPart() rather than the stdlib's
// (*multipart.Reader).ReadForm, which spills large parts to temp files on
// disk above a memory threshold - everything here is read fully in memory,
// consistent with how the rest of this runtime treats bodies.
//
// contentType must carry a "multipart/form-data" media type with a
// boundary parameter; anything else (missing header, wrong media type, no
// boundary) is a plain error - callers turn that into a rejected promise,
// matching how Request/Response's other body-parsing methods (json(),
// e.g.) already reject with a bare error message rather than a typed
// exception.
func parseMultipartFormData(vmInstance *vm.VM, contentType string, data []byte) (vm.Value, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return vm.Value{}, errors.New("Failed to parse body as FormData: could not parse Content-Type")
	}
	if !strings.EqualFold(mediaType, "multipart/form-data") {
		return vm.Value{}, errors.New("Failed to parse body as FormData: Content-Type is not multipart/form-data")
	}
	boundary, ok := params["boundary"]
	if !ok || boundary == "" {
		return vm.Value{}, errors.New("Failed to parse body as FormData: missing boundary in Content-Type")
	}

	fd := &FormData{entries: make([]FormDataEntry, 0)}
	reader := multipart.NewReader(bytes.NewReader(data), boundary)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return vm.Value{}, errors.New("Failed to parse body as FormData: " + err.Error())
		}

		content, err := io.ReadAll(part)
		part.Close()
		if err != nil {
			return vm.Value{}, errors.New("Failed to parse body as FormData: " + err.Error())
		}

		name := part.FormName()
		filename := part.FileName()

		var value vm.Value
		if filename != "" {
			mimeType := blobTypeFromContentType(part.Header.Get("Content-Type"))
			value = NewBlobValue(vmInstance, content, mimeType)
		} else {
			value = vm.NewString(string(content))
		}

		fd.entries = append(fd.entries, FormDataEntry{
			name:     name,
			value:    value,
			filename: filename,
		})
	}

	return NewFormDataValue(vmInstance, fd), nil
}
