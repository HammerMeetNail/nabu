## Description

<!-- Describe the change and its motivation. -->

## Client parity

<!-- Check all that apply. If a client is not affected, explain why in the PR description. -->

- [ ] PWA updated (`web/static/js/` and/or `web/templates/`)
- [ ] iOS updated (`ios/`)
- [ ] PWA-only change (iOS not affected because: ________________)
- [ ] iOS-only change (PWA not affected because: ________________)

## Testing

<!-- Describe how the change was tested. -->

- [ ] Go tests pass (`make test-go`)
- [ ] JS tests pass (`make test-js`)
- [ ] E2E tests pass (`make e2e`)
- [ ] iOS unit tests pass (`xcodebuild test -project ios/Nabu.xcodeproj -scheme Nabu`)
- [ ] iOS UI tests pass (`xcodebuild test -project ios/Nabu.xcodeproj -scheme NabuUITests`)
- [ ] New tests added for changed behavior

## Security

<!-- Confirm security invariants are preserved. -->

- [ ] Ownership/authorization checks preserved
- [ ] User-controlled content escaped as plain text
- [ ] No secrets or credentials in the change
- [ ] Server-side validation preserved for all user inputs
