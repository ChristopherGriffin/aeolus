package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/conditions"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
	"github.com/ChristopherGriffin/aeolus/internal/journey"
)

// clientJourney is one Wi-Fi client's history (0103): GET
// /v1/clients/{mac}?hours=, 24 unless set, at most 720, from the state
// reports of the APs the caller may view.
func (s *Server) clientJourney(w http.ResponseWriter, r *http.Request, c call) error {
	mac := strings.ToLower(r.PathValue("mac"))
	if !macRE.MatchString(mac) {
		return badRequest("a client is named by its MAC, such as aa:bb:cc:00:11:22")
	}
	hours := 24
	if h := r.URL.Query().Get("hours"); h != "" {
		n, err := strconv.Atoi(h)
		if err != nil || n < 1 || n > 720 {
			return badRequest("hours is a whole number from 1 to 720")
		}
		hours = n
	}
	t := c.state.Org.Locations
	may := map[hierarchy.NodeID]bool{}
	var samples []journey.Sample
	err := s.conds.StatesWith(time.Now().Add(-time.Duration(hours)*time.Hour), mac, func(ap hierarchy.NodeID, st conditions.State) error {
		ok, seen := may[ap]
		if !seen {
			ok = roleOn(c, change.Locations, t, ap) >= access.Viewer
			may[ap] = ok
		}
		if !ok {
			return nil
		}
		var rep struct {
			Clients []journey.Client `json:"clients"`
		}
		if json.Unmarshal(st.Report, &rep) != nil {
			return nil
		}
		name := string(ap)
		if n, ok := t.Node(ap); ok {
			name = n.Name
		}
		for _, cl := range rep.Clients {
			if strings.EqualFold(cl.MAC, mac) {
				samples = append(samples, journey.Sample{AP: ap, APName: name, At: st.At, C: cl})
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, journey.Build(mac, samples))
	return nil
}
