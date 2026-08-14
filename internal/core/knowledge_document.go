package core

import (
	"archive/zip"
	"bytes"
	"fmt"
	"html"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

var markupTag = regexp.MustCompile(`<[^>]+>`)

func ExtractKnowledgeText(filename string, data []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".pdf":
		r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return "", fmt.Errorf("parse PDF: %w", err)
		}
		plain, err := r.GetPlainText()
		if err != nil {
			return "", fmt.Errorf("extract PDF text: %w", err)
		}
		b, err := io.ReadAll(io.LimitReader(plain, 64<<20))
		if err != nil {
			return "", err
		}
		return cleanText(string(b)), nil
	case ".docx", ".pptx", ".xlsx", ".odt", ".odp", ".ods":
		return extractOfficeXML(data)
	case ".html", ".htm", ".xml":
		return cleanText(string(data)), nil
	default:
		if !utf8.Valid(data) {
			return "", fmt.Errorf("unsupported binary document %s", ext)
		}
		return cleanText(string(data)), nil
	}
}

func extractOfficeXML(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("open office document: %w", err)
	}
	files := append([]*zip.File(nil), zr.File...)
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	var out strings.Builder
	for _, f := range files {
		name := strings.ToLower(f.Name)
		if !strings.HasSuffix(name, ".xml") || !(strings.HasPrefix(name, "word/") || strings.HasPrefix(name, "ppt/slides/") || strings.HasPrefix(name, "xl/sharedstrings") || strings.HasPrefix(name, "content.xml")) {
			continue
		}
		r, openErr := f.Open()
		if openErr != nil {
			return "", openErr
		}
		b, readErr := io.ReadAll(io.LimitReader(r, 32<<20))
		_ = r.Close()
		if readErr != nil {
			return "", readErr
		}
		out.WriteString(cleanText(string(b)))
		out.WriteByte('\n')
	}
	text := strings.TrimSpace(out.String())
	if text == "" {
		return "", fmt.Errorf("document contains no extractable text")
	}
	return text, nil
}

func cleanText(s string) string {
	s = strings.NewReplacer("</p>", "\n", "</w:p>", "\n", "</a:p>", "\n", "<br>", "\n", "<br/>", "\n").Replace(s)
	s = html.UnescapeString(markupTag.ReplaceAllString(s, " "))
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.Join(strings.Fields(line), " ")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func ChunkKnowledgeText(text string, size, overlap int) []string {
	if size <= 0 {
		size = 1200
	}
	if overlap < 0 || overlap >= size {
		overlap = 200
	}
	r := []rune(strings.TrimSpace(text))
	if len(r) == 0 {
		return nil
	}
	out := []string{}
	for start := 0; start < len(r); {
		end := start + size
		if end > len(r) {
			end = len(r)
		}
		chunk := strings.TrimSpace(string(r[start:end]))
		if chunk != "" {
			out = append(out, chunk)
		}
		if end == len(r) {
			break
		}
		start = end - overlap
	}
	return out
}
