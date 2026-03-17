package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var version = "dev"

const apiEndpoint = "https://api.linear.app/graphql"
const defaultHTTPTimeout = 30 * time.Second

var httpClient = &http.Client{Timeout: defaultHTTPTimeout}

func main() {
	if err := run(os.Args[1:]); err != nil {
		printJSONError(err)
		os.Exit(1)
	}
}

func run(argv []string) error {
	if len(argv) == 0 || argv[0] == "help" {
		printUsage()
		return nil
	}

	cmd := argv[0]
	args := argv[1:]

	switch cmd {
	case "version":
		return printJSON(map[string]any{"version": version})
	case "me":
		return runMe(args)
	case "teams":
		return runTeams(args)
	case "projects":
		return runProjects(args)
	case "issues":
		return runIssues(args)
	case "issue":
		return runIssue(args)
	case "create":
		return runCreate(args)
	case "update":
		return runUpdate(args)
	case "comment":
		return runComment(args)
	case "comments":
		return runComments(args)
	case "comment-update":
		return runCommentUpdate(args)
	case "labels":
		return runLabels(args)
	case "label-create":
		return runLabelCreate(args)
	case "states":
		return runStates(args)
	case "state-create":
		return runStateCreate(args)
	case "state-archive":
		return runStateArchive(args)
	case "upload":
		return runUpload(args)
	case "graphql":
		return runGraphQL(args)
	default:
		return fmt.Errorf("unknown command %q — run 't-linear help' for usage", cmd)
	}
}

// --- GraphQL client ---

type gqlRequest struct {
	Query     string `json:"query"`
	Variables any    `json:"variables,omitempty"`
}

type gqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func apiKey() (string, error) {
	key := os.Getenv("LINEAR_API_KEY")
	if key == "" {
		return "", fmt.Errorf("LINEAR_API_KEY environment variable is not set")
	}
	return key, nil
}

func gql(query string, vars any) (json.RawMessage, error) {
	key, err := apiKey()
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(gqlRequest{Query: query, Variables: vars})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", apiEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", key)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var gqlResp gqlResponse
	if err := json.Unmarshal(respBody, &gqlResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		msgs := make([]string, len(gqlResp.Errors))
		for i, e := range gqlResp.Errors {
			msgs[i] = e.Message
		}
		return nil, fmt.Errorf("GraphQL error: %s", strings.Join(msgs, "; "))
	}

	return gqlResp.Data, nil
}

// gqlField extracts a top-level field from a gql data response into dest.
func gqlField(data json.RawMessage, field string, dest any) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	raw, ok := m[field]
	if !ok {
		return fmt.Errorf("field %q not found in response", field)
	}
	return json.Unmarshal(raw, dest)
}

// --- Commands ---

func runMe(args []string) error {
	data, err := gql(`query { viewer { id name email admin active } }`, nil)
	if err != nil {
		return err
	}
	var user any
	if err := gqlField(data, "viewer", &user); err != nil {
		return err
	}
	return printJSON(user)
}

func runTeams(args []string) error {
	data, err := gql(`query {
		teams {
			nodes {
				id name key description
			}
		}
	}`, nil)
	if err != nil {
		return err
	}
	var teams struct {
		Nodes []any `json:"nodes"`
	}
	if err := gqlField(data, "teams", &teams); err != nil {
		return err
	}
	return printJSON(teams.Nodes)
}

func runProjects(args []string) error {
	fs := flag.NewFlagSet("projects", flag.ContinueOnError)
	team := fs.String("team", "", "filter by team key")
	if err := fs.Parse(args); err != nil {
		return err
	}

	q := `query($filter: ProjectFilter) {
		projects(filter: $filter) {
			nodes {
				id name slugId state
				teams { nodes { key } }
			}
		}
	}`

	var vars map[string]any
	if *team != "" {
		vars = map[string]any{
			"filter": map[string]any{
				"accessibleTeams": map[string]any{
					"some": map[string]any{
						"key": map[string]any{"eq": *team},
					},
				},
			},
		}
	}

	data, err := gql(q, vars)
	if err != nil {
		return err
	}
	var projects struct {
		Nodes []any `json:"nodes"`
	}
	if err := gqlField(data, "projects", &projects); err != nil {
		return err
	}
	return printJSON(projects.Nodes)
}

func runIssues(args []string) error {
	fs := flag.NewFlagSet("issues", flag.ContinueOnError)
	project := fs.String("project", "", "filter by project slug")
	state := fs.String("state", "", "filter by state name(s), comma-separated")
	team := fs.String("team", "", "filter by team key")
	limit := fs.Int("limit", 50, "max results")
	if err := fs.Parse(args); err != nil {
		return err
	}

	filter := map[string]any{}
	if *team != "" {
		filter["team"] = map[string]any{
			"key": map[string]any{"eq": *team},
		}
	}
	if *project != "" {
		filter["project"] = map[string]any{
			"slugId": map[string]any{"eq": *project},
		}
	}
	if *state != "" {
		states := strings.Split(*state, ",")
		for i := range states {
			states[i] = strings.TrimSpace(states[i])
		}
		filter["state"] = map[string]any{
			"name": map[string]any{"in": states},
		}
	}

	q := `query($first: Int, $filter: IssueFilter) {
		issues(first: $first, filter: $filter, orderBy: createdAt) {
			nodes {
				id identifier title priority
				state { name }
				labels { nodes { name } }
				project { slugId name }
				team { key }
				assignee { name email }
				branchName url
				createdAt updatedAt
			}
		}
	}`

	vars := map[string]any{"first": *limit}
	if len(filter) > 0 {
		vars["filter"] = filter
	}

	data, err := gql(q, vars)
	if err != nil {
		return err
	}

	var issues struct {
		Nodes []json.RawMessage `json:"nodes"`
	}
	if err := gqlField(data, "issues", &issues); err != nil {
		return err
	}

	normalized := make([]any, len(issues.Nodes))
	for i, raw := range issues.Nodes {
		n, err := normalizeIssue(raw)
		if err != nil {
			return err
		}
		normalized[i] = n
	}
	return printJSON(normalized)
}

func runIssue(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: t-linear issue <identifier>")
	}
	id := args[0]

	q := `query($id: String!) {
		issue(id: $id) {
			id identifier title description priority
			state { name }
			labels { nodes { name } }
			project { slugId name }
			team { key }
			assignee { name email }
			relations { nodes { type relatedIssue { identifier title state { name } } } }
			branchName url
			createdAt updatedAt
		}
	}`

	data, err := gql(q, map[string]any{"id": id})
	if err != nil {
		return err
	}

	var raw json.RawMessage
	if err := gqlField(data, "issue", &raw); err != nil {
		return err
	}
	n, err := normalizeIssue(raw)
	if err != nil {
		return err
	}
	return printJSON(n)
}

func runCreate(args []string) error {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	team := fs.String("team", "", "team key (required)")
	title := fs.String("title", "", "issue title (required)")
	desc := fs.String("description", "", "issue description")
	project := fs.String("project", "", "project slug")
	priority := fs.Int("priority", 0, "priority (0=none, 1=urgent, 2=high, 3=medium, 4=low)")
	label := fs.String("label", "", "label name(s), comma-separated")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *team == "" || *title == "" {
		return fmt.Errorf("--team and --title are required")
	}

	// Resolve team key to ID
	teamID, err := resolveTeamID(*team)
	if err != nil {
		return err
	}

	input := map[string]any{
		"teamId": teamID,
		"title":  *title,
	}
	if *desc != "" {
		input["description"] = *desc
	}
	if *priority > 0 {
		input["priority"] = *priority
	}
	if *project != "" {
		projectID, err := resolveProjectID(*project)
		if err != nil {
			return err
		}
		input["projectId"] = projectID
	}
	if *label != "" {
		labelIDs, err := resolveLabelIDs(*team, *label)
		if err != nil {
			return err
		}
		input["labelIds"] = labelIDs
	}

	q := `mutation($input: IssueCreateInput!) {
		issueCreate(input: $input) {
			success
			issue {
				id identifier title url
				state { name }
			}
		}
	}`

	data, err := gql(q, map[string]any{"input": input})
	if err != nil {
		return err
	}
	var result struct {
		Success bool `json:"success"`
		Issue   any  `json:"issue"`
	}
	if err := gqlField(data, "issueCreate", &result); err != nil {
		return err
	}
	return printJSON(result)
}

func runUpdate(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: t-linear update <identifier> [flags]")
	}
	identifier := args[0]

	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	state := fs.String("state", "", "new state name")
	priority := fs.Int("priority", -1, "new priority (0=none, 1=urgent, 2=high, 3=medium, 4=low)")
	title := fs.String("title", "", "new title")
	assignee := fs.String("assignee", "", "assignee email")
	addLabel := fs.String("add-label", "", "label name(s) to add, comma-separated")
	removeLabel := fs.String("remove-label", "", "label name(s) to remove, comma-separated")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	issueID, err := resolveIssueID(identifier)
	if err != nil {
		return err
	}

	input := map[string]any{}
	if *state != "" {
		stateID, err := resolveStateID(identifier, *state)
		if err != nil {
			return err
		}
		input["stateId"] = stateID
	}
	if *priority >= 0 {
		input["priority"] = *priority
	}
	if *title != "" {
		input["title"] = *title
	}
	if *assignee != "" {
		userID, err := resolveUserID(*assignee)
		if err != nil {
			return err
		}
		input["assigneeId"] = userID
	}

	// Handle label add/remove: fetch current labels, compute new set
	if *addLabel != "" || *removeLabel != "" {
		newLabelIDs, err := computeLabelUpdate(identifier, issueID, *addLabel, *removeLabel)
		if err != nil {
			return err
		}
		input["labelIds"] = newLabelIDs
	}

	if len(input) == 0 {
		return fmt.Errorf("no updates specified — use --state, --priority, --title, --assignee, --add-label, or --remove-label")
	}

	q := `mutation($id: String!, $input: IssueUpdateInput!) {
		issueUpdate(id: $id, input: $input) {
			success
			issue {
				id identifier title url
				state { name }
				labels { nodes { name } }
			}
		}
	}`

	data, err := gql(q, map[string]any{"id": issueID, "input": input})
	if err != nil {
		return err
	}
	var result struct {
		Success bool `json:"success"`
		Issue   any  `json:"issue"`
	}
	if err := gqlField(data, "issueUpdate", &result); err != nil {
		return err
	}
	return printJSON(result)
}

// computeLabelUpdate fetches the issue's current labels and the team's available labels,
// then computes the new label ID set after adding/removing the specified labels.
func computeLabelUpdate(identifier, issueID, addStr, removeStr string) ([]string, error) {
	// Fetch the issue's current label IDs and team's available labels in one query
	data, err := gql(`query($id: String!) {
		issue(id: $id) {
			labels { nodes { id name } }
			team {
				labels { nodes { id name } }
			}
		}
	}`, map[string]any{"id": issueID})
	if err != nil {
		return nil, err
	}

	var issue struct {
		Labels struct {
			Nodes []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"nodes"`
		} `json:"labels"`
		Team struct {
			Labels struct {
				Nodes []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"nodes"`
			} `json:"labels"`
		} `json:"team"`
	}
	if err := gqlField(data, "issue", &issue); err != nil {
		return nil, err
	}

	// Build team label name→ID map
	teamLabelMap := map[string]string{}
	for _, l := range issue.Team.Labels.Nodes {
		teamLabelMap[strings.ToLower(l.Name)] = l.ID
	}

	// Start with current label IDs
	labelSet := map[string]bool{}
	for _, l := range issue.Labels.Nodes {
		labelSet[l.ID] = true
	}

	// Add labels
	if addStr != "" {
		for _, name := range strings.Split(addStr, ",") {
			name = strings.TrimSpace(name)
			id, ok := teamLabelMap[strings.ToLower(name)]
			if !ok {
				return nil, fmt.Errorf("label %q not found in team — available: %s", name, availableLabelNames(issue.Team.Labels.Nodes))
			}
			labelSet[id] = true
		}
	}

	// Remove labels
	if removeStr != "" {
		for _, name := range strings.Split(removeStr, ",") {
			name = strings.TrimSpace(name)
			id, ok := teamLabelMap[strings.ToLower(name)]
			if !ok {
				// Label doesn't exist in team — silently skip removal
				continue
			}
			delete(labelSet, id)
		}
	}

	ids := make([]string, 0, len(labelSet))
	for id := range labelSet {
		ids = append(ids, id)
	}
	return ids, nil
}

func availableLabelNames(labels []struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}) string {
	names := make([]string, len(labels))
	for i, l := range labels {
		names[i] = l.Name
	}
	return strings.Join(names, ", ")
}

func runComment(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: t-linear comment <identifier> <text>")
	}
	identifier := args[0]
	body := strings.Join(args[1:], " ")

	issueID, err := resolveIssueID(identifier)
	if err != nil {
		return err
	}

	q := `mutation($input: CommentCreateInput!) {
		commentCreate(input: $input) {
			success
			comment { id body createdAt }
		}
	}`

	data, err := gql(q, map[string]any{
		"input": map[string]any{
			"issueId": issueID,
			"body":    body,
		},
	})
	if err != nil {
		return err
	}
	var result struct {
		Success bool `json:"success"`
		Comment any  `json:"comment"`
	}
	if err := gqlField(data, "commentCreate", &result); err != nil {
		return err
	}
	return printJSON(result)
}

func runComments(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: t-linear comments <identifier>")
	}
	identifier := args[0]

	issueID, err := resolveIssueID(identifier)
	if err != nil {
		return err
	}

	q := `query($id: String!) {
		issue(id: $id) {
			comments {
				nodes {
					id body
					user { id name }
					createdAt updatedAt
				}
			}
		}
	}`

	data, err := gql(q, map[string]any{"id": issueID})
	if err != nil {
		return err
	}

	var issue struct {
		Comments struct {
			Nodes []struct {
				ID        string `json:"id"`
				Body      string `json:"body"`
				User      *struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"user"`
				CreatedAt string `json:"createdAt"`
				UpdatedAt string `json:"updatedAt"`
			} `json:"nodes"`
		} `json:"comments"`
	}
	if err := gqlField(data, "issue", &issue); err != nil {
		return err
	}

	normalized := make([]map[string]any, len(issue.Comments.Nodes))
	for i, c := range issue.Comments.Nodes {
		entry := map[string]any{
			"id":         c.ID,
			"body":       c.Body,
			"created_at": c.CreatedAt,
			"updated_at": c.UpdatedAt,
		}
		if c.User != nil {
			entry["user"] = map[string]any{"id": c.User.ID, "name": c.User.Name}
		}
		normalized[i] = entry
	}
	return printJSON(normalized)
}

func runCommentUpdate(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: t-linear comment-update <comment-id> <body>")
	}
	commentID := args[0]
	body := strings.Join(args[1:], " ")

	q := `mutation($id: String!, $input: CommentUpdateInput!) {
		commentUpdate(id: $id, input: $input) {
			success
			comment { id body updatedAt }
		}
	}`

	data, err := gql(q, map[string]any{
		"id":    commentID,
		"input": map[string]any{"body": body},
	})
	if err != nil {
		return err
	}
	var result struct {
		Success bool `json:"success"`
		Comment any  `json:"comment"`
	}
	if err := gqlField(data, "commentUpdate", &result); err != nil {
		return err
	}
	return printJSON(result)
}

func runLabels(args []string) error {
	fs := flag.NewFlagSet("labels", flag.ContinueOnError)
	team := fs.String("team", "", "team key (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *team == "" {
		return fmt.Errorf("--team is required")
	}

	data, err := gql(`query($key: String!) {
		teams(filter: { key: { eq: $key } }) {
			nodes {
				labels {
					nodes { id name color description }
				}
			}
		}
	}`, map[string]any{"key": *team})
	if err != nil {
		return err
	}
	var teams struct {
		Nodes []struct {
			Labels struct {
				Nodes []any `json:"nodes"`
			} `json:"labels"`
		} `json:"nodes"`
	}
	if err := gqlField(data, "teams", &teams); err != nil {
		return err
	}
	if len(teams.Nodes) == 0 {
		return fmt.Errorf("team %q not found", *team)
	}
	return printJSON(teams.Nodes[0].Labels.Nodes)
}

func runLabelCreate(args []string) error {
	fs := flag.NewFlagSet("label-create", flag.ContinueOnError)
	team := fs.String("team", "", "team key (required)")
	name := fs.String("name", "", "label name (required)")
	color := fs.String("color", "#95a2b3", "hex color")
	desc := fs.String("description", "", "label description")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *team == "" || *name == "" {
		return fmt.Errorf("--team and --name are required")
	}

	teamID, err := resolveTeamID(*team)
	if err != nil {
		return err
	}

	input := map[string]any{
		"teamId": teamID,
		"name":   *name,
		"color":  *color,
	}
	if *desc != "" {
		input["description"] = *desc
	}

	q := `mutation($input: IssueLabelCreateInput!) {
		issueLabelCreate(input: $input) {
			success
			issueLabel { id name color }
		}
	}`

	data, err := gql(q, map[string]any{"input": input})
	if err != nil {
		return err
	}
	var result struct {
		Success    bool `json:"success"`
		IssueLabel any  `json:"issueLabel"`
	}
	if err := gqlField(data, "issueLabelCreate", &result); err != nil {
		return err
	}
	return printJSON(result)
}

func runStates(args []string) error {
	fs := flag.NewFlagSet("states", flag.ContinueOnError)
	team := fs.String("team", "", "team key (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *team == "" {
		return fmt.Errorf("--team is required")
	}

	data, err := gql(`query($key: String!) {
		teams(filter: { key: { eq: $key } }) {
			nodes {
				states {
					nodes { id name type position color description }
				}
			}
		}
	}`, map[string]any{"key": *team})
	if err != nil {
		return err
	}
	var teams struct {
		Nodes []struct {
			States struct {
				Nodes []any `json:"nodes"`
			} `json:"states"`
		} `json:"nodes"`
	}
	if err := gqlField(data, "teams", &teams); err != nil {
		return err
	}
	if len(teams.Nodes) == 0 {
		return fmt.Errorf("team %q not found", *team)
	}
	return printJSON(teams.Nodes[0].States.Nodes)
}

func runStateCreate(args []string) error {
	fs := flag.NewFlagSet("state-create", flag.ContinueOnError)
	team := fs.String("team", "", "team key (required)")
	name := fs.String("name", "", "state name (required)")
	stateType := fs.String("type", "", "state type: backlog|unstarted|started|completed|canceled|triage (required)")
	color := fs.String("color", "#95a2b3", "hex color")
	desc := fs.String("description", "", "state description")
	position := fs.Float64("position", -1, "sort position within type category")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *team == "" || *name == "" || *stateType == "" {
		return fmt.Errorf("--team, --name, and --type are required")
	}

	validTypes := map[string]bool{
		"backlog": true, "unstarted": true, "started": true,
		"completed": true, "canceled": true, "triage": true,
	}
	if !validTypes[*stateType] {
		return fmt.Errorf("invalid --type %q — must be one of: backlog, unstarted, started, completed, canceled, triage", *stateType)
	}

	teamID, err := resolveTeamID(*team)
	if err != nil {
		return err
	}

	input := map[string]any{
		"teamId": teamID,
		"name":   *name,
		"type":   *stateType,
		"color":  *color,
	}
	if *desc != "" {
		input["description"] = *desc
	}
	if *position >= 0 {
		input["position"] = *position
	}

	q := `mutation($input: WorkflowStateCreateInput!) {
		workflowStateCreate(input: $input) {
			success
			workflowState { id name type position color description }
		}
	}`

	data, err := gql(q, map[string]any{"input": input})
	if err != nil {
		return err
	}
	var result struct {
		Success       bool `json:"success"`
		WorkflowState any  `json:"workflowState"`
	}
	if err := gqlField(data, "workflowStateCreate", &result); err != nil {
		return err
	}
	return printJSON(result)
}

func runStateArchive(args []string) error {
	fs := flag.NewFlagSet("state-archive", flag.ContinueOnError)
	team := fs.String("team", "", "team key (required)")
	name := fs.String("name", "", "state name to archive (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *team == "" || *name == "" {
		return fmt.Errorf("--team and --name are required")
	}

	// Fetch team states to find the ID
	data, err := gql(`query($key: String!) {
		teams(filter: { key: { eq: $key } }) {
			nodes {
				states {
					nodes { id name }
				}
			}
		}
	}`, map[string]any{"key": *team})
	if err != nil {
		return err
	}
	var teams struct {
		Nodes []struct {
			States struct {
				Nodes []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"nodes"`
			} `json:"states"`
		} `json:"nodes"`
	}
	if err := gqlField(data, "teams", &teams); err != nil {
		return err
	}
	if len(teams.Nodes) == 0 {
		return fmt.Errorf("team %q not found", *team)
	}

	lower := strings.ToLower(*name)
	var stateID string
	for _, s := range teams.Nodes[0].States.Nodes {
		if strings.ToLower(s.Name) == lower {
			stateID = s.ID
			break
		}
	}
	if stateID == "" {
		return fmt.Errorf("state %q not found in team %s", *name, *team)
	}

	archiveData, err := gql(`mutation($id: String!) {
		workflowStateArchive(id: $id) {
			success
		}
	}`, map[string]any{"id": stateID})
	if err != nil {
		return err
	}
	var result struct {
		Success bool `json:"success"`
	}
	if err := gqlField(archiveData, "workflowStateArchive", &result); err != nil {
		return err
	}
	return printJSON(map[string]any{
		"success": result.Success,
		"name":    *name,
		"id":      stateID,
	})
}

func runUpload(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: t-linear upload <file>")
	}
	filePath := args[0]

	// Read file
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Determine content type from extension
	ext := strings.ToLower(filePath)
	if i := strings.LastIndex(ext, "."); i >= 0 {
		ext = ext[i:]
	}
	contentTypes := map[string]string{
		".png":  "image/png",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".gif":  "image/gif",
		".webp": "image/webp",
		".svg":  "image/svg+xml",
	}
	contentType, ok := contentTypes[ext]
	if !ok {
		return fmt.Errorf("unsupported file type %q — supported: png, jpg, jpeg, gif, webp, svg", ext)
	}

	// Extract filename from path
	filename := filePath
	if i := strings.LastIndexAny(filename, "/\\"); i >= 0 {
		filename = filename[i+1:]
	}

	// Step 1: Request upload URL from Linear
	q := `mutation($contentType: String!, $filename: String!, $size: Int!) {
		fileUpload(contentType: $contentType, filename: $filename, size: $size) {
			uploadFile {
				uploadUrl
				assetUrl
			}
		}
	}`

	data, err := gql(q, map[string]any{
		"contentType": contentType,
		"filename":    filename,
		"size":        len(fileData),
	})
	if err != nil {
		return err
	}

	var result struct {
		UploadFile struct {
			UploadURL string `json:"uploadUrl"`
			AssetURL  string `json:"assetUrl"`
		} `json:"uploadFile"`
	}
	if err := gqlField(data, "fileUpload", &result); err != nil {
		return err
	}

	// Step 2: PUT the file to the upload URL
	req, err := http.NewRequest("PUT", result.UploadFile.UploadURL, bytes.NewReader(fileData))
	if err != nil {
		return fmt.Errorf("create upload request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Cache-Control", "public, max-age=31536000")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload HTTP %d: %s", resp.StatusCode, string(body))
	}

	return printJSON(map[string]any{
		"asset_url": result.UploadFile.AssetURL,
		"filename":  filename,
	})
}

func runGraphQL(args []string) error {
	fs := flag.NewFlagSet("graphql", flag.ContinueOnError)
	query := fs.String("query", "", "GraphQL query string")
	varsStr := fs.String("vars", "", "variables as JSON string")
	if err := fs.Parse(args); err != nil {
		return err
	}

	q := *query
	if q == "" {
		// Read from stdin
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		q = string(b)
	}
	if q == "" {
		return fmt.Errorf("no query provided — use --query or pipe via stdin")
	}

	var vars any
	if *varsStr != "" {
		if err := json.Unmarshal([]byte(*varsStr), &vars); err != nil {
			return fmt.Errorf("parse --vars: %w", err)
		}
	}

	data, err := gql(q, vars)
	if err != nil {
		return err
	}

	var parsed any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	return printJSON(parsed)
}

// --- Resolvers ---

func resolveTeamID(key string) (string, error) {
	data, err := gql(`query($key: String!) {
		teams(filter: { key: { eq: $key } }) {
			nodes { id }
		}
	}`, map[string]any{"key": key})
	if err != nil {
		return "", err
	}
	var teams struct {
		Nodes []struct {
			ID string `json:"id"`
		} `json:"nodes"`
	}
	if err := gqlField(data, "teams", &teams); err != nil {
		return "", err
	}
	if len(teams.Nodes) == 0 {
		return "", fmt.Errorf("team %q not found", key)
	}
	return teams.Nodes[0].ID, nil
}

func resolveProjectID(slug string) (string, error) {
	data, err := gql(`query {
		projects { nodes { id slugId } }
	}`, nil)
	if err != nil {
		return "", err
	}
	var projects struct {
		Nodes []struct {
			ID     string `json:"id"`
			SlugID string `json:"slugId"`
		} `json:"nodes"`
	}
	if err := gqlField(data, "projects", &projects); err != nil {
		return "", err
	}
	for _, p := range projects.Nodes {
		if p.SlugID == slug {
			return p.ID, nil
		}
	}
	return "", fmt.Errorf("project with slug %q not found", slug)
}

func resolveIssueID(identifier string) (string, error) {
	data, err := gql(`query($id: String!) {
		issue(id: $id) { id }
	}`, map[string]any{"id": identifier})
	if err != nil {
		return "", err
	}
	var issue struct {
		ID string `json:"id"`
	}
	if err := gqlField(data, "issue", &issue); err != nil {
		return "", err
	}
	return issue.ID, nil
}

func resolveStateID(identifier string, stateName string) (string, error) {
	// Get the team from the issue, then find the state
	data, err := gql(`query($id: String!) {
		issue(id: $id) {
			team {
				states { nodes { id name } }
			}
		}
	}`, map[string]any{"id": identifier})
	if err != nil {
		return "", err
	}
	var issue struct {
		Team struct {
			States struct {
				Nodes []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"nodes"`
			} `json:"states"`
		} `json:"team"`
	}
	if err := gqlField(data, "issue", &issue); err != nil {
		return "", err
	}
	lower := strings.ToLower(stateName)
	for _, s := range issue.Team.States.Nodes {
		if strings.ToLower(s.Name) == lower {
			return s.ID, nil
		}
	}
	names := make([]string, len(issue.Team.States.Nodes))
	for i, s := range issue.Team.States.Nodes {
		names[i] = s.Name
	}
	return "", fmt.Errorf("state %q not found — available: %s", stateName, strings.Join(names, ", "))
}

func resolveUserID(email string) (string, error) {
	data, err := gql(`query {
		users { nodes { id email } }
	}`, nil)
	if err != nil {
		return "", err
	}
	var users struct {
		Nodes []struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		} `json:"nodes"`
	}
	if err := gqlField(data, "users", &users); err != nil {
		return "", err
	}
	for _, u := range users.Nodes {
		if strings.EqualFold(u.Email, email) {
			return u.ID, nil
		}
	}
	return "", fmt.Errorf("user with email %q not found", email)
}

func resolveLabelIDs(teamKey string, labelStr string) ([]string, error) {
	data, err := gql(`query($key: String!) {
		teams(filter: { key: { eq: $key } }) {
			nodes {
				labels { nodes { id name } }
			}
		}
	}`, map[string]any{"key": teamKey})
	if err != nil {
		return nil, err
	}
	var teams struct {
		Nodes []struct {
			Labels struct {
				Nodes []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"nodes"`
			} `json:"labels"`
		} `json:"nodes"`
	}
	if err := gqlField(data, "teams", &teams); err != nil {
		return nil, err
	}
	if len(teams.Nodes) == 0 {
		return nil, fmt.Errorf("team %q not found", teamKey)
	}

	labelMap := map[string]string{}
	for _, l := range teams.Nodes[0].Labels.Nodes {
		labelMap[strings.ToLower(l.Name)] = l.ID
	}

	names := strings.Split(labelStr, ",")
	ids := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		id, ok := labelMap[strings.ToLower(name)]
		if !ok {
			return nil, fmt.Errorf("label %q not found in team %s", name, teamKey)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// --- Issue normalization ---

func normalizeIssue(raw json.RawMessage) (map[string]any, error) {
	var issue struct {
		ID          string `json:"id"`
		Identifier  string `json:"identifier"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Priority    int    `json:"priority"`
		State       *struct {
			Name string `json:"name"`
		} `json:"state"`
		Labels *struct {
			Nodes []struct {
				Name string `json:"name"`
			} `json:"nodes"`
		} `json:"labels"`
		Project *struct {
			SlugID string `json:"slugId"`
			Name   string `json:"name"`
		} `json:"project"`
		Team *struct {
			Key string `json:"key"`
		} `json:"team"`
		Assignee *struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"assignee"`
		Relations *struct {
			Nodes []struct {
				Type         string `json:"type"`
				RelatedIssue struct {
					Identifier string `json:"identifier"`
					Title      string `json:"title"`
					State      *struct {
						Name string `json:"name"`
					} `json:"state"`
				} `json:"relatedIssue"`
			} `json:"nodes"`
		} `json:"relations"`
		BranchName string `json:"branchName"`
		URL        string `json:"url"`
		CreatedAt  string `json:"createdAt"`
		UpdatedAt  string `json:"updatedAt"`
	}
	if err := json.Unmarshal(raw, &issue); err != nil {
		return nil, err
	}

	out := map[string]any{
		"id":          issue.ID,
		"identifier":  issue.Identifier,
		"title":       issue.Title,
		"description": issue.Description,
		"priority":    issue.Priority,
		"branch_name": issue.BranchName,
		"url":         issue.URL,
		"created_at":  issue.CreatedAt,
		"updated_at":  issue.UpdatedAt,
	}

	if issue.State != nil {
		out["state"] = issue.State.Name
	}
	if issue.Team != nil {
		out["team"] = issue.Team.Key
	}
	if issue.Assignee != nil {
		out["assignee"] = map[string]any{"name": issue.Assignee.Name, "email": issue.Assignee.Email}
	}
	if issue.Project != nil {
		out["project"] = map[string]any{"slug": issue.Project.SlugID, "name": issue.Project.Name}
	}

	labels := []string{}
	if issue.Labels != nil {
		for _, l := range issue.Labels.Nodes {
			labels = append(labels, strings.ToLower(l.Name))
		}
	}
	out["labels"] = labels

	blockedBy := []map[string]any{}
	if issue.Relations != nil {
		for _, r := range issue.Relations.Nodes {
			if r.Type == "blocks" {
				entry := map[string]any{
					"identifier": r.RelatedIssue.Identifier,
					"title":      r.RelatedIssue.Title,
				}
				if r.RelatedIssue.State != nil {
					entry["state"] = r.RelatedIssue.State.Name
				}
				blockedBy = append(blockedBy, entry)
			}
		}
	}
	out["blocked_by"] = blockedBy

	return out, nil
}

// --- JSON helpers ---

func writeJSON(v any) error {
	pretty := strings.TrimSpace(os.Getenv("T_LINEAR_PRETTY")) == "1"
	var (
		b   []byte
		err error
	)
	if pretty {
		b, err = json.MarshalIndent(v, "", "  ")
	} else {
		b, err = json.Marshal(v)
	}
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func printJSON(v any) error {
	envelope := strings.TrimSpace(os.Getenv("T_LINEAR_ENVELOPE")) == "1"
	if envelope {
		return writeJSON(map[string]any{"ok": true, "data": v})
	}
	return writeJSON(v)
}

func printJSONError(err error) {
	_ = writeJSON(map[string]any{
		"ok": false,
		"error": map[string]any{
			"code":    "FATAL",
			"message": err.Error(),
		},
	})
}

// --- Usage ---

func printUsage() {
	fmt.Println("t-linear — Linear CLI")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  me                                   Show current user")
	fmt.Println("  teams                                List teams")
	fmt.Println("  projects [--team KEY]                List projects")
	fmt.Println("  issues [--project SLUG] [--state S]  List issues")
	fmt.Println("         [--team KEY] [--limit N]")
	fmt.Println("  issue <ID>                           Show issue detail")
	fmt.Println("  create --team KEY --title T          Create issue")
	fmt.Println("         [--description D] [--project SLUG]")
	fmt.Println("         [--priority N] [--label L]")
	fmt.Println("  update <ID> [--state S] [--priority N]")
	fmt.Println("              [--title T] [--assignee EMAIL]")
	fmt.Println("              [--add-label L] [--remove-label L]")
	fmt.Println("  comment <ID> <text>                  Add comment")
	fmt.Println("  comments <ID>                        List comments on issue")
	fmt.Println("  comment-update <CID> <body>          Update a comment by ID")
	fmt.Println("  labels --team KEY                    List team labels")
	fmt.Println("  label-create --team KEY --name N     Create label")
	fmt.Println("               [--color HEX]")
	fmt.Println("               [--description D]")
	fmt.Println("  states --team KEY                    List workflow states")
	fmt.Println("  state-create --team KEY --name N     Create workflow state")
	fmt.Println("               --type T [--color HEX]")
	fmt.Println("               [--description D] [--position N]")
	fmt.Println("  state-archive --team KEY --name N    Archive workflow state")
	fmt.Println("  upload <file>                        Upload image, return asset URL")
	fmt.Println("  graphql [--query Q] [--vars JSON]    Raw GraphQL")
	fmt.Println("  version                              Print version")
	fmt.Println()
	fmt.Println("Auth: set LINEAR_API_KEY environment variable")
	fmt.Println()
	fmt.Println("Env vars:")
	fmt.Println("  T_LINEAR_PRETTY=1    Pretty-print JSON output")
	fmt.Println("  T_LINEAR_ENVELOPE=1  Wrap output in {ok, data} envelope")
}
