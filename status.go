package main

import (
	"bytes"
	"html/template"
	"io"
	"net/http"

	"github.com/stapelberg/hmgo/internal/hm"
)

const statusTmplContents = `
<!DOCTYPE html>
<title>hmgo</title>
<body>
<h1>Devices</h1>
<table width="100%">
{{ range $serial, $dev := .Devices }}
<tr>
<td>{{ $serial }}</td>
<td>
{{ $dev }}<br>
{{ range $idx, $event := $dev.MostRecentEvents }}
<ul>
{{ $event.HTML }}
</ul>
{{ end }}
</td>
</tr>
{{ end }}
</table>
`

var statusTmpl = template.Must(template.New("status").Parse(statusTmplContents))

func handleStatus(w http.ResponseWriter, r *http.Request, devs map[string]hm.Device) {
	var buf bytes.Buffer

	if err := statusTmpl.Execute(&buf, struct {
		Devices map[string]hm.Device
	}{
		Devices: devs,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	io.Copy(w, &buf)
}
