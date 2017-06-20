package instrumentation

import (
	"net/http"
	"testing"

	"github.com/fd/httpmiddlewarevet"
)

func TestResponseWriterConformance(t *testing.T) {
	httpmiddlewarevet.Vet(t, func(h http.Handler) http.Handler {
		return InstrumentHandler("testing", h)
	})
}
