package gorpc

import (
	"fmt"
	"html/template"
	"net/http"
)

const debugText = `<html>
	<body>
	<title>GeeRPC Services</title>
	{{range .}}
	<hr>
	Service {{.Name}}
	<hr>
		<table>
		<th align=center>Method</th><th align=center>Calls</th>
		{{range $name, $mtype := .Method}}
			<tr>
			<td align=left font=fixed>{{$name}}({{$mtype.ArgType}}, {{$mtype.ReplyType}}) error</td>
			<td align=center>{{$mtype.NumCalls}}</td>
			</tr>
		{{end}}
		</table>
	{{end}}
	</body>
	</html>`

var debug = template.Must(template.New("debug").Parse(debugText))

type debugHTTP struct {
	*Server
}

type debugService struct {
	Name   string
	Method map[string]*MethodType
}

func (server debugHTTP) ServeHTTP(w http.ResponseWriter, respons *http.Request) {
	var services []debugService
	server.serviceMap.Range(func(namei, servicei interface{}) bool {
		service := servicei.(*service)
		services = append(services, debugService{
			Name:   namei.(string),
			Method: service.method,
		})
		return true
	})
	err := debug.Execute(w, services)
	if err != nil {
		fmt.Fprintln(w, "fail to execute template:", err.Error())
	}
}
