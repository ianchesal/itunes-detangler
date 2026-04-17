package classifier

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
)

var errBoxNotFound = errors.New("mp4: box not found")

type boxHeader struct {
	size    uint32 // total box size including the 8-byte header
	boxType string
}

func readBoxHeader(r io.Reader) (boxHeader, error) {
	var buf [8]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return boxHeader{}, err
	}
	return boxHeader{
		size:    binary.BigEndian.Uint32(buf[:4]),
		boxType: string(buf[4:8]),
	}, nil
}

// findBox scans for a box of the given type within limit bytes of the current
// reader position. On success r is positioned at the start of that box's content.
// Returns the content size (box size minus the 8-byte header).
// Note: ISO base media allows size==1 (64-bit largesize follows) and size==0
// (box extends to EOF). Neither appears at moov/stsd level in iTunes files,
// so both are treated as errBoxNotFound via the size < 8 guard.
func findBox(r io.ReadSeeker, target string, limit int64) (int64, error) {
	var consumed int64
	for consumed < limit {
		h, err := readBoxHeader(r)
		if err != nil {
			return 0, err
		}
		if h.size < 8 {
			return 0, errBoxNotFound
		}
		consumed += int64(h.size)
		if h.boxType == target {
			return int64(h.size) - 8, nil
		}
		if _, err := r.Seek(int64(h.size)-8, io.SeekCurrent); err != nil {
			return 0, err
		}
	}
	return 0, errBoxNotFound
}

func classifyM4A(path string) (Category, error) {
	f, err := os.Open(path)
	if err != nil {
		return CategorySkip, err
	}
	defer f.Close()

	totalSize, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return CategorySkip, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return CategorySkip, err
	}
	return classifyM4AReader(f, totalSize)
}

// classifyM4AReader is the testable core of classifyM4A.
func classifyM4AReader(r io.ReadSeeker, totalSize int64) (Category, error) {
	moovSize, err := findBox(r, "moov", totalSize)
	if err != nil {
		return CategorySkip, nil // not a valid MP4 — skip silently
	}
	trakSize, err := findBox(r, "trak", moovSize)
	if err != nil {
		return CategorySkip, nil
	}
	mdiaSize, err := findBox(r, "mdia", trakSize)
	if err != nil {
		return CategorySkip, nil
	}
	minfSize, err := findBox(r, "minf", mdiaSize)
	if err != nil {
		return CategorySkip, nil
	}
	stblSize, err := findBox(r, "stbl", minfSize)
	if err != nil {
		return CategorySkip, nil
	}
	if _, err := findBox(r, "stsd", stblSize); err != nil {
		return CategorySkip, nil
	}

	// Skip stsd preamble: version(1) + flags(3) + entryCount(4) = 8 bytes
	if _, err := r.Seek(8, io.SeekCurrent); err != nil {
		return CategorySkip, nil
	}

	entryHdr, err := readBoxHeader(r)
	if err != nil {
		return CategorySkip, nil
	}
	entryContentSize := int64(entryHdr.size) - 8

	switch entryHdr.boxType {
	case "alac":
		return CategoryDRMFree, nil
	case "mp4a":
		// Skip AudioSampleEntry fixed fields (28 bytes) before any child boxes.
		// Fields: reserved(6)+dataRefIdx(2)+reserved(8)+channels(2)+sampleSize(2)+
		//         compressionId(2)+packetSize(2)+sampleRate(4) = 28 bytes
		if _, err := r.Seek(28, io.SeekCurrent); err != nil {
			return CategorySkip, nil
		}
		_, sinfErr := findBox(r, "sinf", entryContentSize-28)
		if errors.Is(sinfErr, errBoxNotFound) {
			return CategoryDRMFree, nil
		}
		if sinfErr != nil {
			return CategorySkip, nil
		}
		return CategoryDRMProtected, nil
	default:
		return CategorySkip, nil
	}
}
