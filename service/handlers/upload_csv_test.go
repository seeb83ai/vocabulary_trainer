package handlers_test

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"vocabulary_trainer/db"
	"vocabulary_trainer/handlers"
	"vocabulary_trainer/models"

	"github.com/go-chi/chi/v5"
)

func doMultipart(t *testing.T, r http.Handler, path string, fields map[string]string, fileContent string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatalf("write field %s: %v", k, err)
		}
	}
	if fileContent != "" {
		fw, err := w.CreateFormFile("file", "words.csv")
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := fw.Write([]byte(fileContent)); err != nil {
			t.Fatalf("write file content: %v", err)
		}
	}
	w.Close()
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// uploadRouterWithLimits builds a minimal authenticated router whose CSV handler
// uses the given DoS limits, so the body-size and row caps can be exercised
// without constructing multi-megabyte payloads.
func uploadRouterWithLimits(s *db.Store, maxBytes int64, maxRows int) http.Handler {
	h := &handlers.UploadCSVHandler{Store: s, MaxBytes: maxBytes, MaxRows: maxRows}
	r := chi.NewRouter()
	r.Use(handlers.WithUserID(2))
	r.Post("/api/words/upload-csv", h.UploadCSV)
	return r
}

func TestUploadCSV_NoFile(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := doMultipart(t, r, "/api/words/upload-csv", map[string]string{"tags": "test"}, "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUploadCSV_MissingTags(t *testing.T) {
	csv := "chinese,pinyin,en\n我,wǒ,I"
	r := newRouter(openTestDB(t))
	rec := doMultipart(t, r, "/api/words/upload-csv", map[string]string{}, csv)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUploadCSV_BadCSVHeader(t *testing.T) {
	csv := "word,pinyin,en\n我,wǒ,I"
	r := newRouter(openTestDB(t))
	rec := doMultipart(t, r, "/api/words/upload-csv", map[string]string{"tags": "test"}, csv)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUploadCSV_ValidBasic(t *testing.T) {
	csv := "chinese,pinyin,en\n我要回家了,wǒ yào huí jiā le,I go home"
	r := newRouter(openTestDB(t))
	rec := doMultipart(t, r, "/api/words/upload-csv",
		map[string]string{"tags": "test", "start_training_count": "0"}, csv)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)
	if resp["imported"] != 1 {
		t.Errorf("want imported=1, got %d", resp["imported"])
	}
	if resp["updated"] != 0 {
		t.Errorf("want updated=0, got %d", resp["updated"])
	}
	if resp["total"] != 1 {
		t.Errorf("want total=1, got %d", resp["total"])
	}
}

func TestUploadCSV_DuplicateCallsUpdate(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "我要回家了", "wǒ yào huí jiā le", []string{"old translation"})
	csv := "chinese,pinyin,en\n我要回家了,wǒ yào huí jiā le,I go home"
	r := newRouter(s)
	rec := doMultipart(t, r, "/api/words/upload-csv",
		map[string]string{"tags": "test", "start_training_count": "0"}, csv)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)
	if resp["imported"] != 0 {
		t.Errorf("want imported=0, got %d", resp["imported"])
	}
	if resp["updated"] != 1 {
		t.Errorf("want updated=1, got %d", resp["updated"])
	}
}

func TestUploadCSV_MultipleLanguages(t *testing.T) {
	csv := "chinese,pinyin,en,de\n你好,nǐ hǎo,hello,Hallo"
	r := newRouter(openTestDB(t))
	rec := doMultipart(t, r, "/api/words/upload-csv",
		map[string]string{"tags": "test", "start_training_count": "0"}, csv)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)
	if resp["imported"] != 1 {
		t.Errorf("want imported=1, got %d", resp["imported"])
	}
}

func TestUploadCSV_NoPinyinColumn(t *testing.T) {
	csv := "chinese,en\n我要回家了,I go home\n你好,hello"
	r := newRouter(openTestDB(t))
	rec := doMultipart(t, r, "/api/words/upload-csv",
		map[string]string{"tags": "test", "start_training_count": "0"}, csv)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 for CSV without pinyin column, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)
	if resp["imported"] != 2 {
		t.Errorf("want imported=2, got %d", resp["imported"])
	}
}

func TestUploadCSV_NoPinyinMultipleLangs(t *testing.T) {
	csv := "chinese,en,de\n你好,hello,Hallo"
	r := newRouter(openTestDB(t))
	rec := doMultipart(t, r, "/api/words/upload-csv",
		map[string]string{"tags": "test", "start_training_count": "0"}, csv)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 for CSV without pinyin column with multiple langs, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)
	if resp["imported"] != 1 {
		t.Errorf("want imported=1, got %d", resp["imported"])
	}
}

func TestUploadCSV_AutoGeneratesPinyinWhenColumnAbsent(t *testing.T) {
	csv := "chinese,en\n你好,hello"
	r := newRouter(openTestDB(t))
	rec := doMultipart(t, r, "/api/words/upload-csv",
		map[string]string{"tags": "test", "start_training_count": "0"}, csv)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var upload map[string]int
	decodeJSON(t, rec, &upload)
	if upload["imported"] != 1 {
		t.Fatalf("want imported=1, got %d", upload["imported"])
	}

	rec2 := do(t, r, "GET", "/api/words?page=1&per_page=20", nil)
	var resp models.WordListResponse
	decodeJSON(t, rec2, &resp)
	if len(resp.Words) != 1 {
		t.Fatalf("want 1 word in list, got %d", len(resp.Words))
	}
	wd := resp.Words[0]
	if wd.Pinyin == nil || *wd.Pinyin == "" {
		t.Errorf("expected pinyin to be auto-generated for %q, got nil/empty", wd.ZhText)
	}
}

func TestUploadCSV_MultipleSemicolonTranslations(t *testing.T) {
	csv := "chinese,pinyin,de\n我要回家了,wǒ yào huí jiā le,Ich gehe nach Hause; Ich gehe heim"
	r := newRouter(openTestDB(t))
	rec := doMultipart(t, r, "/api/words/upload-csv",
		map[string]string{"tags": "test", "start_training_count": "0"}, csv)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)
	if resp["imported"] != 1 {
		t.Errorf("want imported=1, got %d", resp["imported"])
	}
}

func TestUploadCSV_StartTraining(t *testing.T) {
	csv := "chinese,pinyin,en\n一,yī,one\n二,èr,two\n三,sān,three"
	r := newRouter(openTestDB(t))
	rec := doMultipart(t, r, "/api/words/upload-csv",
		map[string]string{"tags": "test", "start_training_count": "2"}, csv)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)
	if resp["total"] != 3 {
		t.Errorf("want total=3, got %d", resp["total"])
	}
}

func TestUploadCSV_RejectsOversizedBody(t *testing.T) {
	// Tiny body cap; the multipart payload below exceeds it.
	r := uploadRouterWithLimits(openTestDB(t), 500, 5000)
	var b strings.Builder
	b.WriteString("chinese,pinyin,en\n")
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&b, "字%d,zì,meaning number %d here padding padding\n", i, i)
	}
	rec := doMultipart(t, r, "/api/words/upload-csv",
		map[string]string{"tags": "t", "start_training_count": "0"}, b.String())
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413 for oversized body, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUploadCSV_RejectsTooManyRows(t *testing.T) {
	// Row cap of 2; the CSV below has 3 data rows.
	r := uploadRouterWithLimits(openTestDB(t), 0, 2)
	csv := "chinese,pinyin,en\n一,yī,one\n二,èr,two\n三,sān,three"
	rec := doMultipart(t, r, "/api/words/upload-csv",
		map[string]string{"tags": "t", "start_training_count": "0"}, csv)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for too many rows, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "too many rows") {
		t.Errorf("expected a 'too many rows' message, got %s", rec.Body.String())
	}
}
