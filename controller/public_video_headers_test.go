package controller

import (
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestPublicVideoHeadersPreserveRangeWithoutProviderDetails(t *testing.T) {
	source := http.Header{
		"Content-Length": {"4"}, "Content-Range": {"bytes 0-3/100"}, "Accept-Ranges": {"bytes"},
		"X-Processing-Stage": {"private-worker"}, "Content-Location": {"https://supplier.invalid/private.mp4"},
		"Content-Disposition": {"attachment; filename=internal.mp4"}, "Set-Cookie": {"private=session"},
		"Etag": {"private-model"}, "Content-Type": {"video/mp4; stage=private"},
	}
	dst := http.Header{}
	copyPublicVideoHeaders(dst, source)
	require.Equal(t, "4", dst.Get("Content-Length"))
	require.Equal(t, "bytes 0-3/100", dst.Get("Content-Range"))
	require.Equal(t, "bytes", dst.Get("Accept-Ranges"))
	for _, key := range []string{"X-Processing-Stage", "Content-Location", "Content-Disposition", "Set-Cookie", "Etag", "Content-Type"} {
		require.Empty(t, dst.Get(key))
	}
}
