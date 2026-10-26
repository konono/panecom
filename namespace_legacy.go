package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// legacyRegistrations finds live roles in this session's store whose recorded
// anchor is inside the current project but differs from its canonical anchor.
// It only reads state; moving registrations requires the owner to register
// again, so role conflicts and links are never silently overwritten.
func legacyRegistrations(sRoot, session, anchor string) []legacyRegistration {
	entries, err := os.ReadDir(filepath.Join(sRoot, "namespaces"))
	if err != nil {
		return nil
	}
	home := canonicalHome()
	currentHash := hashString(anchor)
	currentNs := namespaceDir(sRoot, anchor)
	var out []legacyRegistration
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == currentHash {
			continue
		}
		nsDir := filepath.Join(sRoot, "namespaces", entry.Name())
		oldAnchor, err := readFile(filepath.Join(nsDir, ".cwd"))
		if err != nil || oldAnchor == "" || hashString(oldAnchor) != entry.Name() {
			continue
		}
		if namespaceAnchorFrom(oldAnchor, home) != anchor {
			continue
		}
		for _, role := range listRoles(session, nsDir) {
			if role.Alive {
				old := legacyRegistration{Anchor: oldAnchor, Role: role.Role, PaneID: role.PaneID}
				if occupied, err := resolveRole(currentNs, role.Role); err == nil && occupied != role.PaneID {
					alive, checkErr := mux.PaneExists(session, occupied)
					if checkErr != nil || alive {
						old.ConflictPaneID = occupied
					}
				}
				out = append(out, old)
			}
		}
	}
	return out
}

type legacyRegistration struct {
	Anchor         string
	Role           string
	PaneID         string
	ConflictPaneID string
}

func warnLegacyRegistrations(w io.Writer, sRoot, session, anchor string) {
	for _, old := range legacyRegistrations(sRoot, session, anchor) {
		if old.ConflictPaneID != "" {
			fmt.Fprintf(w, "panecom: old namespace %s has %s (%s), but it belongs to %s here; choose a different role before re-registering\n", old.Anchor, old.Role, old.PaneID, old.ConflictPaneID)
			continue
		}
		fmt.Fprintf(w, "panecom: old namespace %s still has %s (%s); with that pane in this project, run 'panecom register %s' there or select it with 'panecom register' from its tab\n", old.Anchor, old.Role, old.PaneID, old.Role)
	}
}
