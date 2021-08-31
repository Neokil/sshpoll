package sshio

import (
	"fmt"
	"io"
	"strings"
)

func ReadLine(rw io.ReadWriter) (string, error) {
	result := ""
	buf := make([]byte, 1)

	for {
		_, err := rw.Read(buf)
		if err != nil {
			return "", fmt.Errorf("could not read from session: %w", err)
		}
		switch buf[0] {
		case 127: // backspace
			if result != "" {
				result = result[:len(result)-1]
				_, err := io.WriteString(rw, "\b \b")
				if err != nil {
					return "", fmt.Errorf("could not write to session: %w", err)
				}
			}
		case 13: // enter
			_, err := io.WriteString(rw, "\n")
			if err != nil {
				return "", fmt.Errorf("could not write to session: %w", err)
			}
			return result, nil
		default:
			_, err := io.WriteString(rw, string(buf))
			if err != nil {
				return "", fmt.Errorf("could not write to session: %w", err)
			}
			result += string(buf)
		}
	}
}

func ReadKey(r io.Reader) (byte, error) {
	buf := make([]byte, 1)
	_, err := r.Read(buf)
	if err != nil {
		return buf[0], fmt.Errorf("could not read from session: %w", err)
	}
	return buf[0], nil
}

func NewPage(w io.Writer, width int, height int) error {
	_, err := io.WriteString(w, strings.Repeat(strings.Repeat(" ", width)+"\n", height))
	if err != nil {
		return fmt.Errorf("could not write to session: %w", err)
	}
	return nil
}
