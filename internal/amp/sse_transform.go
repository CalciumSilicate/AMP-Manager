package amp

import (
	"bytes"
	"io"
)

type sseTransformWrapper struct {
	rc        io.ReadCloser
	buf       []byte
	out       bytes.Buffer
	transform func([]byte) []byte
	eof       bool
}

func NewSSETransformWrapper(rc io.ReadCloser, transform func([]byte) []byte) io.ReadCloser {
	if rc == nil {
		return nil
	}
	if transform == nil {
		return rc
	}
	return &sseTransformWrapper{rc: rc, transform: transform}
}

func (w *sseTransformWrapper) Close() error {
	return w.rc.Close()
}

func (w *sseTransformWrapper) Read(p []byte) (int, error) {
	if w.out.Len() > 0 {
		return w.out.Read(p)
	}

	if w.eof {
		if len(w.buf) > 0 {
			w.out.Write(w.transformSSEFrame(w.buf))
			w.buf = nil
			return w.out.Read(p)
		}
		return 0, io.EOF
	}

	tmp := make([]byte, 8*1024)
	n, err := w.rc.Read(tmp)
	if n > 0 {
		w.buf = append(w.buf, tmp[:n]...)
	}
	if err == io.EOF {
		w.eof = true
	} else if err != nil {
		return 0, err
	}

	for {
		idx, delimLen := findSSEDelimiter(w.buf)
		if idx < 0 {
			break
		}
		frame := w.buf[:idx+delimLen]
		w.buf = w.buf[idx+delimLen:]
		w.out.Write(w.transformSSEFrame(frame))
	}

	if w.out.Len() > 0 {
		return w.out.Read(p)
	}

	if w.eof {
		return w.Read(p)
	}
	return 0, nil
}

func (w *sseTransformWrapper) transformSSEFrame(frame []byte) []byte {
	// Transform only data: JSON lines; keep other lines intact.
	lines := bytes.Split(frame, []byte("\n"))
	for i, line := range lines {
		hadCR := bytes.HasSuffix(line, []byte("\r"))
		core := line
		if hadCR {
			core = core[:len(core)-1]
		}
		if bytes.HasPrefix(core, []byte("data:")) {
			payload := bytes.TrimSpace(core[len("data:"):])
			if len(payload) > 0 && (payload[0] == '{' || payload[0] == '[') {
				payload = w.transform(payload)
				core = append([]byte("data: "), payload...)
			}
		}
		if hadCR {
			core = append(core, '\r')
		}
		lines[i] = core
	}
	return bytes.Join(lines, []byte("\n"))
}

type sseJSONTransformWrapper struct {
	rc        io.ReadCloser
	buf       []byte
	out       bytes.Buffer
	transform func([]byte) [][]byte
	eof       bool
}

func NewSSEJSONTransformWrapper(rc io.ReadCloser, transform func([]byte) [][]byte) io.ReadCloser {
	if rc == nil {
		return nil
	}
	if transform == nil {
		return rc
	}
	return &sseJSONTransformWrapper{rc: rc, transform: transform}
}

func (w *sseJSONTransformWrapper) Close() error {
	return w.rc.Close()
}

func (w *sseJSONTransformWrapper) Read(p []byte) (int, error) {
	if w.out.Len() > 0 {
		return w.out.Read(p)
	}

	if w.eof {
		if len(w.buf) > 0 {
			w.out.Write(w.transformSSEFrame(w.buf))
			w.buf = nil
			return w.out.Read(p)
		}
		return 0, io.EOF
	}

	tmp := make([]byte, 8*1024)
	n, err := w.rc.Read(tmp)
	if n > 0 {
		w.buf = append(w.buf, tmp[:n]...)
	}
	if err == io.EOF {
		w.eof = true
	} else if err != nil {
		return 0, err
	}

	for {
		idx, delimLen := findSSEDelimiter(w.buf)
		if idx < 0 {
			break
		}
		frame := w.buf[:idx+delimLen]
		w.buf = w.buf[idx+delimLen:]
		w.out.Write(w.transformSSEFrame(frame))
	}

	if w.out.Len() > 0 {
		return w.out.Read(p)
	}

	if w.eof {
		return w.Read(p)
	}
	return 0, nil
}

func (w *sseJSONTransformWrapper) transformSSEFrame(frame []byte) []byte {
	lines := bytes.Split(frame, []byte("\n"))
	for _, line := range lines {
		core := bytes.TrimSuffix(line, []byte("\r"))
		if !bytes.HasPrefix(core, []byte("data:")) {
			continue
		}

		payload := bytes.TrimSpace(core[len("data:"):])
		if len(payload) == 0 || (payload[0] != '{' && payload[0] != '[') {
			continue
		}

		transformedPayloads := w.transform(payload)
		if len(transformedPayloads) == 0 {
			return frame
		}

		var out bytes.Buffer
		for _, transformed := range transformedPayloads {
			out.WriteString("data: ")
			out.Write(transformed)
			out.WriteString("\n\n")
		}
		return out.Bytes()
	}

	return frame
}
