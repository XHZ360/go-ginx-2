## ADDED Requirements

### Requirement: Administrator session endpoint baseline
The system SHALL expose dedicated same-origin administrator session endpoints for the separated admin console.

#### Scenario: Login creates an administrator browser session
- **WHEN** a valid administrator submits credentials to the separated admin-console login endpoint
- **THEN** the system verifies those credentials against the protected administrator credential source, creates a server-managed administrator session, sets a browser session cookie, and returns the minimal bootstrap information needed for the frontend shell

#### Scenario: Session bootstrap returns current auth context
- **WHEN** the separated frontend calls the administrator session bootstrap endpoint with a valid browser session
- **THEN** the system returns the minimal authenticated administrator context needed for route guards, shell initialization, and CSRF-aware follow-up requests

#### Scenario: Logout invalidates the administrator browser session
- **WHEN** the separated frontend calls the administrator logout endpoint for a current browser session
- **THEN** the system invalidates the corresponding server-managed session and clears the browser session cookie

### Requirement: Administrator session lifecycle baseline
The system SHALL enforce lifecycle rules for separated-console administrator sessions.

#### Scenario: Session expiry rejects further separated-console access
- **WHEN** an administrator browser session is missing, expired, or invalid
- **THEN** the separated-console session bootstrap endpoint and session-authenticated API operations reject access without exposing protected administrator resources

#### Scenario: Process restart invalidates in-memory administrator sessions
- **WHEN** the server process restarts while the first separated-console session implementation uses in-memory session storage
- **THEN** previously issued administrator sessions are no longer valid and administrators must authenticate again

### Requirement: Browser mutation CSRF baseline
The system SHALL protect session-authenticated separated-console mutations against CSRF.

#### Scenario: Session-authenticated mutation requires a valid CSRF token
- **WHEN** a separated-console browser mutation uses a valid administrator session
- **THEN** the system requires a valid CSRF token in addition to the session cookie before allowing the mutation to proceed

#### Scenario: Session-authenticated query access does not require CSRF
- **WHEN** a separated-console browser request performs a session-authenticated read-only operation
- **THEN** the system may allow that request without a CSRF token as long as the administrator session is valid

### Requirement: API-only administrator browser surface baseline
The system SHALL stop serving the legacy server-rendered administrator UI once the session-authenticated admin surface is introduced.

#### Scenario: Server-rendered administrator routes are removed
- **WHEN** the session-authenticated administrator API surface is introduced
- **THEN** the legacy server-rendered administrator pages and browser-facing form handlers are no longer served

#### Scenario: Removed administrator page paths return not found
- **WHEN** a browser requests removed administrator page paths such as `/`, `/users`, `/clients`, `/proxies`, `/certificates`, or `/audit` on the admin listener after this slice lands
- **THEN** the admin listener returns `404 Not Found` instead of serving HTML or redirecting to a replacement UI that does not yet exist

#### Scenario: Separated-console API routes use administrator session auth
- **WHEN** the separated admin frontend calls the new same-origin admin API routes
- **THEN** those routes authenticate administrators through the server-managed session model rather than repeated Basic Auth prompts

#### Scenario: Legacy browser-facing GraphQL route is removed
- **WHEN** the session-authenticated administrator API surface is introduced
- **THEN** the legacy browser-facing `POST /graphql` route is no longer served for administrator browser access and browser clients use the session-authenticated GraphQL entrypoint instead

## MODIFIED Requirements

### Requirement: Administrator authentication baseline
The system SHALL protect the separated management frontend and API with administrator-only authentication that is separate from client credentials.

#### Scenario: Browser login establishes a session from file-backed administrator credentials
- **WHEN** a valid administrator signs in to the separated management console
- **THEN** the management plane verifies that administrator against the protected administrator credential source and establishes a server-managed browser session over TLS rather than relying on repeated HTTP Basic Auth prompts

#### Scenario: Legacy browser admin flow is not retained as a fallback
- **WHEN** the administrator session model is introduced for the browser-facing management surface
- **THEN** the system does not retain the server-rendered administrator UI or repeated browser-facing Basic Auth as a fallback path

### Requirement: Session bootstrap baseline
The system SHALL provide a browser-session bootstrap contract for the separated administrator console.

#### Scenario: Bootstrap returns minimal administrator session context
- **WHEN** the separated frontend loads or reloads a protected route with a valid administrator session
- **THEN** the frontend can query the dedicated session/bootstrap endpoint to recover the minimal current-session context, including the information needed for route guards and follow-up authenticated browser requests

### Requirement: GraphQL admin API gap tracking
The admin-resource-management spec SHALL treat the administrator GraphQL management API as the primary business-data contract for the separated console while continuing to track broader management capabilities as future work.

#### Scenario: Separated console uses session-authenticated GraphQL entrypoint
- **WHEN** an authenticated administrator uses the separated management console
- **THEN** the console reads and mutates the confirmed scoped management resources through the session-authenticated administrator GraphQL entrypoint rather than through repeated Basic Auth browser prompts
