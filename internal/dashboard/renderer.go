package dashboard

import "bytes"

var (
	stylesPlaceholder  = []byte("{{STYLES}}")
	authPlaceholder    = []byte("{{AUTH_SCRIPT}}")
	scriptPlaceholder  = []byte("{{PAGE_SCRIPT}}")
	initialPlaceholder = []byte("{{INITIAL_JSON}}")
)

// renderPage assembles a self-contained resource page. Assets stay separated
// in source code but are inlined in the response because plugin resource
// routes do not expose a static-file server.
func renderPage(pageTemplate, pageScript []byte, data Data) ([]byte, error) {
	initial, err := initialJSON(data)
	if err != nil {
		return nil, err
	}
	page := bytes.ReplaceAll(pageTemplate, stylesPlaceholder, styles)
	page = bytes.ReplaceAll(page, authPlaceholder, managementAuthScript)
	combinedScript := make([]byte, 0, len(localeScript)+len(pageScript)+1)
	combinedScript = append(combinedScript, localeScript...)
	combinedScript = append(combinedScript, '\n')
	combinedScript = append(combinedScript, pageScript...)
	page = bytes.ReplaceAll(page, scriptPlaceholder, combinedScript)
	page = bytes.ReplaceAll(page, initialPlaceholder, []byte(initial))
	return page, nil
}
