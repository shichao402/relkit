package changelog

import rupv2 "cnb.cool/shichao402/relkit/api/rup/v2"

// CompactPriorNodes rewrites older index nodes so only the newest release keeps
// an inlined markdown body. For every other node:
//
//   - if urlTemplate is set, ensure notesUrl is populated from it
//   - clear notes once a notesUrl is available (index size / display contract)
//
// The head node (keepCode) is left untouched.
func CompactPriorNodes(nodes []*rupv2.VersionNode, urlTemplate string, keepCode int64) error {
	if urlTemplate == "" || len(nodes) == 0 {
		return nil
	}
	for _, node := range nodes {
		if node == nil || node.Code == keepCode {
			continue
		}
		if node.NotesUrl == "" {
			u, err := FormatURL(urlTemplate, node.Version)
			if err != nil {
				return err
			}
			node.NotesUrl = u
		}
		if node.NotesUrl != "" {
			node.Notes = ""
		}
	}
	return nil
}
