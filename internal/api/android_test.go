package api

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// The Android app's "Share to Photoframe" builds its multipart body itself
// (apps/android core/Uploads.kt, Multipart), and its own test checks that it
// writes exactly testdata/android-upload.multipart. Feeding that body to the
// real handler here is what keeps the two sides of the upload in step: a
// non-ASCII name, a name with quotes in it, a PNG and a JPEG.
//
// The app also sends no Origin and no Sec-Fetch-Site header (it is not a
// browser), which sameOrigin lets through, as it does curl.
func TestAndroidAppUpload(t *testing.T) {
	body, err := os.Open(filepath.Join("testdata", "android-upload.multipart"))
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()

	e := newEnv(t)
	resp, err := http.Post(e.srv.URL+"/api/sources/uploads/photos",
		"multipart/form-data; boundary=PhotoframeAndroidTestBoundary", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d, want 201", resp.StatusCode)
	}
	res := decode[UploadResult](t, resp)
	want := []string{"Caf_ dot.png", "say _22cheese_22.jpg"}
	if !slices.Equal(res.Saved, want) || len(res.Rejected) != 0 {
		t.Fatalf("upload = %+v, want saved %q", res, want)
	}
	for _, name := range want {
		if _, err := os.Stat(filepath.Join(e.data, "uploads", "uploads", name)); err != nil {
			t.Errorf("%s not stored: %v", name, err)
		}
	}
}
