package buildinfo

// MinimumMacOSMajor is the oldest supported host release. Native builds use the
// same deployment target; embedded helpers must not require a newer system.
const MinimumMacOSMajor = 26
