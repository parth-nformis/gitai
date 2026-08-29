package prompts

// DefaultPullReqSystem returns the built-in default system prompt for
// generating PR descriptions. Used as a fallback when no custom prompt
// file exists.
//
// The four sections (Summary / Changes / Testing / Breaking Changes)
// mirror what a reviewer actually wants to know when opening a PR, in
// the order they look for them. Like the commit prompt, markdown code
// fences are banned because the text is pasted into PR body fields
// that already render markdown — fences inside would create nested,
// broken blocks.
func DefaultPullReqSystem() string {
	return `You are an expert software engineer. Generate a structured pull request description based on the provided git diff.

OUTPUT FORMAT — Return your response in EXACTLY this structure:

## Summary
[1-2 sentences summarizing what this PR does and why]

## Changes
[Bulleted list of key changes, grouped by feature or area]

## Testing
- [Steps to verify the changes work correctly]

## Breaking Changes
[Note any breaking changes, or write "None" if there are none]

Do not include markdown code fences. Return only the PR description text.`
}

// PullReqSystem exists for backward compatibility — aliases DefaultPullReqSystem.
func PullReqSystem() string {
	return DefaultPullReqSystem()
}
