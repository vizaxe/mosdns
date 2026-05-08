package geofile

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxEntrySize = 20 << 20 // 20MB, safe upper bound for a single entry

func LoadIP(file, code string) ([]*CIDR, error) {
	file = resolveDatFilePath(file)
	entry, err := findEntry(file, []byte(strings.ToUpper(code)))
	if err != nil {
		return nil, err
	}
	geoip, err := unmarshalGeoIP(entry)
	if err != nil {
		return nil, err
	}
	return geoip.Cidr, nil
}

func LoadSite(file, code string) ([]*Domain, error) {
	file = resolveDatFilePath(file)
	entry, err := findEntry(file, []byte(strings.ToUpper(code)))
	if err != nil {
		return nil, err
	}
	geosite, err := unmarshalGeoSite(entry)
	if err != nil {
		return nil, err
	}
	return geosite.Domain, nil
}

func resolveDatFilePath(path string) string {
	if path != "geoip" && path != "geosite" {
		return path
	}

	fileName := path + ".dat"

	if dir := os.Getenv("XRAY_LOCATION_ASSET"); dir != "" {
		p := filepath.Join(dir, fileName)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	if dir := os.Getenv("V2RAY_LOCATION_ASSET"); dir != "" {
		p := filepath.Join(dir, fileName)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	dirs := []string{
		"/usr/local/share/xray",
		"/usr/share/xray",
		"/usr/local/share/v2ray",
		"/usr/share/v2ray",
	}
	for _, dir := range dirs {
		p := filepath.Join(dir, fileName)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return path
}

func findEntry(file string, code []byte) ([]byte, error) {
	if _, err := os.Stat(file); err != nil {
		return nil, fmt.Errorf("geo file %s: %w", file, err)
	}

	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	br := bufio.NewReaderSize(f, 65536)

	for {
		_, err := br.ReadByte()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		entryLen, err := binary.ReadUvarint(br)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if entryLen == 0 || entryLen > maxEntrySize {
			continue
		}

		entry := make([]byte, entryLen)
		if _, err := io.ReadFull(br, entry); err != nil {
			return nil, err
		}

		if matchCode(entry, code) {
			return entry, nil
		}
	}

	return nil, fmt.Errorf("code %s not found", string(code))
}

func matchCode(entry, code []byte) bool {
	if len(entry) < 2 || entry[0] != 0x0A {
		return false
	}
	strLen, n := binary.Uvarint(entry[1:])
	if n <= 0 || int(strLen) != len(code) {
		return false
	}
	start := 1 + n
	end := start + int(strLen)
	if end > len(entry) {
		return false
	}
	for i, b := range code {
		if entry[start+i] != b {
			return false
		}
	}
	return true
}
