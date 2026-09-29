package web

import (
	"encoding/json"
	"net/http"

	"github.com/fanama/resume/atscv/internal/fill"
	"github.com/fanama/resume/atscv/internal/model"
)

// importRequest is what the browser sends: the text it read out of a PDF, and
// the language the headings are in when it already knows it.
//
// The PDF itself never reaches the server. Decoding it is mechanical work, it
// is done in the browser where the file already is, and it keeps this endpoint
// as cheap as the rest of the API: a JSON body, a JSON answer, no state, no
// file written, nothing to clean up.
type importRequest struct {
	Text string `json:"text"`
	Lang string `json:"lang"`
}

// importResponse is the draft, plus what the draft could not hold. The
// unmapped headings are reported rather than swallowed: a section the schema
// has no field for is content the candidate wrote and the tool would otherwise
// drop silently.
type importResponse struct {
	Resume   *model.Resume `json:"resume"`
	Lang     string        `json:"lang"`
	Unmapped []string      `json:"unmapped"`
}

// maxImportText bounds the body. A resume is a few thousand characters; a
// generated document can be far more, and the endpoint is public, so a body
// larger than this is a mistake or an attack rather than a CV.
const maxImportText = 512 << 10

// handleImport fills a resume from the text of a PDF. The answer is a draft,
// always: the heuristics are guesses about a page that has already lost its
// structure, and the editor says so before showing it.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImportText)
	var req importRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the body is not a JSON object with a \"text\" field"})
		return
	}
	// A language is a hint, never a contract. When the caller sends one that is
	// not a language this tool has, the document is read instead: importing a
	// French CV into an English editor has to find the French headings, and the
	// editor's own language is not evidence about the PDF.
	lang, err := model.ParseLang(req.Lang)
	if err != nil {
		lang = ""
	}

	result, err := fill.Run(req.Text, lang)
	if err != nil {
		// A PDF with no usable text is a scan. The answer says so instead of
		// returning an empty draft that would look like a successful import.
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, importResponse{
		Resume:   result.Resume,
		Lang:     string(result.Lang),
		Unmapped: result.Unmapped,
	})
}
