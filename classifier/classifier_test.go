package classifier

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyDRMFreeExtensions(t *testing.T) {
	for _, ext := range []string{".mp3", ".flac", ".aiff", ".aif", ".wav"} {
		t.Run(ext, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "*"+ext)
			if err != nil {
				t.Fatal(err)
			}
			f.Close()
			got, err := Classify(f.Name())
			if err != nil {
				t.Fatalf("Classify: unexpected error: %v", err)
			}
			if got != CategoryDRMFree {
				t.Errorf("Classify(%q) = %v, want %v", filepath.Ext(f.Name()), got, CategoryDRMFree)
			}
		})
	}
}

func TestClassifyM4PIsProtected(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.m4p")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	got, err := Classify(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != CategoryDRMProtected {
		t.Errorf("Classify(.m4p) = %v, want CategoryDRMProtected", got)
	}
}

func TestClassifyUnknownExtensionIsSkip(t *testing.T) {
	for _, ext := range []string{".jpg", ".pdf", ".xml", ".nfo"} {
		t.Run(ext, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "*"+ext)
			if err != nil {
				t.Fatal(err)
			}
			f.Close()
			got, err := Classify(f.Name())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != CategorySkip {
				t.Errorf("Classify(%q) = %v, want CategorySkip", ext, got)
			}
		})
	}
}

func TestClassifyIsCaseInsensitive(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.MP3")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	got, err := Classify(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != CategoryDRMFree {
		t.Errorf("Classify(.MP3) = %v, want CategoryDRMFree", got)
	}
}

// --- MP4 test fixture helpers ---

func buildBox(boxType string, content []byte) []byte {
	result := make([]byte, 8+len(content))
	binary.BigEndian.PutUint32(result[:4], uint32(8+len(content)))
	copy(result[4:8], boxType)
	copy(result[8:], content)
	return result
}

func buildAudioEntry(codec string, hasSinf bool) []byte {
	// AudioSampleEntry fixed fields (28 bytes):
	// reserved(6) + dataRefIdx(2) + reserved(8) + channels(2) + sampleSize(2) +
	// compressionId(2) + packetSize(2) + sampleRate(4)
	fields := make([]byte, 28)
	binary.BigEndian.PutUint16(fields[6:8], 1)           // dataRefIdx = 1
	binary.BigEndian.PutUint16(fields[16:18], 2)         // channels = 2
	binary.BigEndian.PutUint16(fields[18:20], 16)        // sampleSize = 16
	binary.BigEndian.PutUint32(fields[24:28], 44100<<16) // sampleRate

	content := append([]byte(nil), fields...)
	if hasSinf {
		content = append(content, buildBox("sinf", []byte{0})...)
	}
	return buildBox(codec, content)
}

func buildM4AData(codec string, hasSinf bool) []byte {
	entry := buildAudioEntry(codec, hasSinf)

	// stsd preamble: version(1)+flags(3)+entryCount(4) = 8 bytes
	stsdContent := make([]byte, 8)
	binary.BigEndian.PutUint32(stsdContent[4:8], 1) // entryCount = 1
	stsdContent = append(stsdContent, entry...)

	stsd := buildBox("stsd", stsdContent)
	stbl := buildBox("stbl", stsd)
	minf := buildBox("minf", stbl)
	mdia := buildBox("mdia", minf)
	trak := buildBox("trak", mdia)
	return buildBox("moov", trak)
}

func writeTempM4A(t *testing.T, data []byte) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*.m4a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

// --- M4A classification tests ---

func TestClassifyM4ADRMFreeAAC(t *testing.T) {
	path := writeTempM4A(t, buildM4AData("mp4a", false))
	got, err := Classify(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != CategoryDRMFree {
		t.Errorf("DRM-free AAC .m4a: got %v, want CategoryDRMFree", got)
	}
}

func TestClassifyM4AProtectedAAC(t *testing.T) {
	path := writeTempM4A(t, buildM4AData("mp4a", true))
	got, err := Classify(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != CategoryDRMProtected {
		t.Errorf("protected AAC .m4a: got %v, want CategoryDRMProtected", got)
	}
}

func TestClassifyM4AAppleLossless(t *testing.T) {
	path := writeTempM4A(t, buildM4AData("alac", false))
	got, err := Classify(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != CategoryDRMFree {
		t.Errorf("Apple Lossless .m4a: got %v, want CategoryDRMFree", got)
	}
}
