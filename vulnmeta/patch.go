package vulnmeta

// Patch is a partial update of a Classification. A nil field leaves that part
// untouched; an empty value clears it: cvss_vector "", epss {}, remediation "",
// framework_tags [].
type Patch struct {
	CVSSVector  *string `json:"cvss_vector,omitempty"`
	EPSS        *EPSS   `json:"epss,omitempty"`
	Remediation *string `json:"remediation,omitempty"`
	Tags        *[]Tag  `json:"framework_tags,omitempty"`
}

// IsEmpty reports whether the patch changes nothing.
func (p Patch) IsEmpty() bool {
	return p.CVSSVector == nil && p.EPSS == nil && p.Remediation == nil && p.Tags == nil
}

// Apply returns base with the patch applied, normalized (see Normalize).
func (p Patch) Apply(base Classification) (Classification, error) {
	next := base
	if p.CVSSVector != nil {
		next.CVSS = &CVSS{Vector: *p.CVSSVector}
	}
	if p.EPSS != nil {
		e := *p.EPSS
		next.EPSS = &e
	}
	if p.Remediation != nil {
		next.Remediation = *p.Remediation
	}
	if p.Tags != nil {
		next.Tags = *p.Tags
	}
	return Normalize(next)
}
