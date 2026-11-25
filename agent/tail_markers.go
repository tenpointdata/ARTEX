package agent

// CodeOwnedTailMarkers are strings of the code-owned prompt tails and the
// lesson/data envelopes. An editable prompt body that contains one could
// imitate or pre-empt a tail, so the RSI gate (server) and the RSI proposer
// (cmd/artex-rsi-propose) both refuse such a body through rsi.ValidateEdit.
var CodeOwnedTailMarkers = []string{
	"## Prior evidence from earlier tasks", // lesson block header (db.RenderLessons)
	"ARTEX_DATA",                           // data envelope marker stem
	"Data envelope (code-owned)",
	"Extra duty: register the test asset scope", // goals scope tail (agent/goals.go)
	"[Noise profile: ",                          // noise profile tail (agent/noise_profile.go)
}
