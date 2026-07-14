#!/usr/bin/env bash
# Linear GraphQL wrapper. No MCP, no deps beyond curl + jq.
# Operations map 1:1 to docs/agents/issue-tracker.md. Default team "Unbusy".
set -euo pipefail

ENDPOINT="https://api.linear.app/graphql"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"   # .agents/skills/linear -> repo root

err() { printf 'linear: %s\n' "$*" >&2; }
die() { err "$*"; exit 1; }

# Resolve LINEAR_API_KEY from env, else the repo's gitignored .env/.env.local.
load_key() {
    [ -n "${LINEAR_API_KEY:-}" ] && return 0
    local f val
    for f in "$REPO_ROOT/.env" "$REPO_ROOT/.env.local" "$HOME/.config/linear.env" "$HOME/.linear.env"; do
        [ -f "$f" ] || continue
        val=$(grep -E '^LINEAR_API_KEY=' "$f" 2>/dev/null | head -1 \
            | sed -E 's/^LINEAR_API_KEY=//; s/^"(.*)"$/\1/; s/^'\''(.*)'\''$/\1/')
        [ -n "$val" ] && { LINEAR_API_KEY="$val"; return 0; }
    done
    die "LINEAR_API_KEY not set. Make one at https://linear.app/settings/api and put 'LINEAR_API_KEY=lin_api_...' in $REPO_ROOT/.env (gitignored)."
}

# POST a GraphQL document. $1 = query string, $2 = variables JSON. Prints .data on stdout.
gql() {
    local query="$1" vars="${2:-{\}}"
    load_key
    local body out code
    body=$(jq -n --arg q "$query" --argjson v "$vars" '{query: $q, variables: $v}')
    out=$(curl -sS -w '\n%{http_code}' -X POST "$ENDPOINT" \
        -H "Authorization: $LINEAR_API_KEY" \
        -H "Content-Type: application/json" \
        -d "$body") || die "HTTP request failed"
    code=$(printf '%s' "$out" | tail -n1)
    out=$(printf '%s' "$out" | sed '$d')
    if [ "$code" -ge 400 ] 2>/dev/null; then
        err "HTTP $code:"; printf '%s\n' "$out" >&2; exit 1
    fi
    if printf '%s' "$out" | jq -e '.errors' >/dev/null 2>&1; then
        err "GraphQL errors:"; printf '%s' "$out" | jq '.errors' >&2; exit 1
    fi
    printf '%s' "$out" | jq '.data'
}

# ---- resolvers ---------------------------------------------------------------

team_id() {  # [name] -> uuid
    local name="${1:-${LINEAR_TEAM:-Unbusy}}"
    local data id
    data=$(gql 'query($n:String!){teams(filter:{name:{eq:$n}}){nodes{id key name}}}' "$(jq -n --arg n "$name" '{n:$n}')") || exit 1
    id=$(printf '%s' "$data" | jq -r --arg name "$name" \
        '.teams.nodes | if length > 0 then .[0].id else error("team not found: " + $name) end') || exit 1
    printf '%s' "$id"
}

# Accepts a UUID or an identifier like UNB-42; prints the UUID.
# Linear's IssueFilter has no `identifier` field, so split <KEY>-<num>
# and resolve via number + team key.
resolve_issue_id() {
    local ref="$1"
    if printf '%s' "$ref" | grep -qEi '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'; then
        printf '%s' "$ref"; return
    fi
    printf '%s' "$ref" | grep -qE '^[A-Za-z]+-[0-9]+$' \
        || die "not a UUID or identifier (expected UNB-42 or a UUID): $ref"
    local key num data id
    key=${ref%%-*}; num=${ref##*-}
    data=$(gql 'query($k:String!,$n:Float!){issues(filter:{number:{eq:$n},team:{key:{eq:$k}}}){nodes{id identifier}}}' \
        "$(jq -n --arg k "$key" --argjson n "$num" '{k:$k,n:$n}')") || exit 1
    id=$(printf '%s' "$data" | jq -r --arg ref "$ref" \
        '.issues.nodes | if length > 0 then .[0].id else error("issue not found: " + $ref) end') || exit 1
    printf '%s' "$id"
}

# <team_id> "A, B" -> ["id","id"]; errors listing available labels if any name is unknown.
label_ids_for() {
    local tid="$1" names="$2"
    local data ids
    data=$(gql 'query($t:String!){team(id:$t){labels{nodes{id name}}}}' "$(jq -n --arg t "$tid" '{t:$t}')") || exit 1
    ids=$(printf '%s' "$data" | jq --arg names "$names" '
        ($names | split(",") | map(gsub("^\\s+|\\s+$"; ""))) as $want
        | .team.labels.nodes as $all
        | ($want | map(. as $w | ($all | map(select(.name == $w)))
            | if length > 0 then .[0].id else null end)) as $got
        | if ($got | any(. == null)) then
            error("unknown label(s): " + ([($want[] | select(. as $w | $all | any(.name == $w) | not))] | join(", "))
                  + " — available: " + ($all | map(.name) | join(", ")))
          else $got end') || exit 1
    printf '%s' "$ids"
}

# <team_id> <state-name> -> id; errors listing available states if unknown.
state_id_for() {
    local tid="$1" name="$2"
    local data id
    data=$(gql 'query($t:String!){team(id:$t){states{nodes{id name type}}}}' "$(jq -n --arg t "$tid" '{t:$t}')") || exit 1
    id=$(printf '%s' "$data" | jq -r --arg name "$name" \
        '.team.states.nodes as $all | ($all | map(select(.name == $name)))
         | if length > 0 then .[0].id else error("unknown state: " + $name
              + " — available: " + ($all | map(.name) | join(", "))) end') || exit 1
    printf '%s' "$id"
}

# ---- commands ----------------------------------------------------------------

cmd_team() {
    local name="${LINEAR_TEAM:-Unbusy}"
    while [ $# -gt 0 ]; do case "$1" in
        --name) name="$2"; shift 2;; *) die "team: unknown arg: $1";; esac; done
    gql 'query($n:String!){teams(filter:{name:{eq:$n}}){nodes{id key name}}}' "$(jq -n --arg n "$name" '{n:$n}')"
}

cmd_create() {
    local title="" desc="" labels=""
    while [ $# -gt 0 ]; do case "$1" in
        --title) title="$2"; shift 2;;
        --description|--desc) desc="$2"; shift 2;;
        --labels) labels="$2"; shift 2;;
        *) die "create: unknown arg: $1";; esac; done
    [ -n "$title" ] || die "create: --title is required"
    local tid; tid=$(team_id)
    local label_ids="[]"
    [ -n "$labels" ] && label_ids=$(label_ids_for "$tid" "$labels")
    local input
    input=$(jq -n --arg team "$tid" --arg title "$title" --arg desc "$desc" --argjson labels "$label_ids" \
        '{ teamId: $team, title: $title, description: $desc }
         + (if ($labels | length) > 0 then { labelIds: $labels } else {} end)')
    gql 'mutation($input:IssueCreateInput!){issueCreate(input:$input){issue{id identifier url}}}' \
        "$(jq -n --argjson i "$input" '{input: $i}')"
}

cmd_get() {
    local ref="${1:-}"
    [ -n "$ref" ] || die "get: issue identifier or id required"
    local id; id=$(resolve_issue_id "$ref")
    gql 'query($id:String!){issue(id:$id){id identifier title description url state{name} labels{nodes{id name}} assignee{name}}}' \
        "$(jq -n --arg id "$id" '{id: $id}')"
}

cmd_list() {
    local state="" label="" assignee=""
    while [ $# -gt 0 ]; do case "$1" in
        --state) state="$2"; shift 2;;
        --label) label="$2"; shift 2;;
        --assignee) assignee="$2"; shift 2;;
        *) die "list: unknown arg: $1";; esac; done
    local tid; tid=$(team_id)
    local filter
    filter=$(jq -n --arg team "$tid" --arg state "$state" --arg label "$label" --arg assignee "$assignee" '
        { team: { id: { eq: $team } } }
        + (if $state != "" then { state: { name: { eq: $state } } } else {} end)
        + (if $label != "" then { labels: { name: { eq: $label } } } else {} end)
        + (if $assignee == "me" then { assignee: { isMe: { eq: true } } }
           elif $assignee != "" then { assignee: { name: { eq: $assignee } } } else {} end)')
    gql 'query($f:IssueFilter){issues(filter:$f,first:50,orderBy:updatedAt){nodes{id identifier title url state{name} assignee{name}}}}' \
        "$(jq -n --argjson f "$filter" '{f: $f}')"
}

cmd_comments() {
    local ref="${1:-}"
    [ -n "$ref" ] || die "comments: issue ref required"
    local id; id=$(resolve_issue_id "$ref")
    gql 'query($id:String!){issue(id:$id){comments(last:50){nodes{createdAt user{name} body}}}}' \
        "$(jq -n --arg id "$id" '{id: $id}')"
}

cmd_comment() {
    local ref="${1:-}" body="${2:-}"
    [ -n "$ref" ] || die "comment: issue ref required"
    [ -n "$body" ] || die "comment: body required (quote it)"
    local id; id=$(resolve_issue_id "$ref")
    local input
    input=$(jq -n --arg issue "$id" --arg body "$body" '{ issueId: $issue, body: $body }')
    gql 'mutation($input:CommentCreateInput!){commentCreate(input:$input){comment{url}}}' \
        "$(jq -n --argjson i "$input" '{input: $i}')"
}

cmd_set_labels() {  # replaces the full set
    local ref="${1:-}" labels="${2:-}"
    [ -n "$ref" ] || die "set-labels: issue ref required"
    local id; id=$(resolve_issue_id "$ref")
    local tid; tid=$(team_id)
    local label_ids="[]"
    [ -n "$labels" ] && label_ids=$(label_ids_for "$tid" "$labels")
    local input
    input=$(jq -n --argjson labels "$label_ids" '{ labelIds: $labels }')
    gql 'mutation($id:String!,$input:IssueUpdateInput!){issueUpdate(id:$id,input:$input){issue{id identifier labels{nodes{name}}}}}' \
        "$(jq -n --arg id "$id" --argjson i "$input" '{id: $id, input: $i}')"
}

cmd_add_labels() {  # unions with existing labels (the safe variant)
    local ref="${1:-}" labels="${2:-}"
    [ -n "$ref" ] || die "add-labels: issue ref required"
    [ -n "$labels" ] || die "add-labels: comma list of labels required"
    local id; id=$(resolve_issue_id "$ref")
    local tid; tid=$(team_id)
    local new_ids cur data cur_ids union input
    new_ids=$(label_ids_for "$tid" "$labels")
    data=$(gql 'query($id:String!){issue(id:$id){labels{nodes{id}}}}' "$(jq -n --arg id "$id" '{id: $id}')") || exit 1
    cur_ids=$(printf '%s' "$data" | jq '.issue.labels.nodes | map(.id)')
    union=$(jq -n --argjson a "$cur_ids" --argjson b "$new_ids" '($a + $b) | unique')
    input=$(jq -n --argjson labels "$union" '{ labelIds: $labels }')
    gql 'mutation($id:String!,$input:IssueUpdateInput!){issueUpdate(id:$id,input:$input){issue{id identifier labels{nodes{name}}}}}' \
        "$(jq -n --arg id "$id" --argjson i "$input" '{id: $id, input: $i}')"
}

cmd_create_label() {
    local name="${1:-}" team_override=""
    [ -n "$name" ] || die "create-label: name required"
    shift
    while [ $# -gt 0 ]; do case "$1" in
        --team) team_override="$2"; shift 2;; *) die "create-label: unknown arg: $1";; esac; done
    local tid; tid=$(team_id ${team_override:+"$team_override"})
    local input
    input=$(jq -n --arg name "$name" --arg team "$tid" '{ name: $name, teamId: $team }')
    gql 'mutation($input:IssueLabelCreateInput!){issueLabelCreate(input:$input){issueLabel{id name team{id key}}}}' \
        "$(jq -n --argjson i "$input" '{input: $i}')"
}

cmd_statuses() {
    local tid; tid=$(team_id)
    gql 'query($t:String!){team(id:$t){states{nodes{id name type}}}}' "$(jq -n --arg t "$tid" '{t: $t}')"
}

cmd_labels() {
    local tid; tid=$(team_id)
    gql 'query($t:String!){team(id:$t){labels{nodes{id name}}}}' "$(jq -n --arg t "$tid" '{t: $t}')"
}

cmd_update() {  # generic; close = update --state <Done/Cancelled name>
    local ref="${1:-}"; [ -n "$ref" ] || die "update: issue ref required"; shift
    local state="" labels=""
    while [ $# -gt 0 ]; do case "$1" in
        --state) state="$2"; shift 2;;
        --labels) labels="$2"; shift 2;;
        *) die "update: unknown arg: $1";; esac; done
    { [ -n "$state" ] || [ -n "$labels" ]; } || die "update: needs --state and/or --labels"
    local id; id=$(resolve_issue_id "$ref")
    local tid; tid=$(team_id)
    local input='{}'
    if [ -n "$state" ]; then
        local sid; sid=$(state_id_for "$tid" "$state")
        input=$(printf '%s' "$input" | jq --arg sid "$sid" '. + { stateId: $sid }')
    fi
    if [ -n "$labels" ]; then
        local lids; lids=$(label_ids_for "$tid" "$labels")
        input=$(printf '%s' "$input" | jq --argjson lids "$lids" '. + { labelIds: $lids }')
    fi
    gql 'mutation($id:String!,$input:IssueUpdateInput!){issueUpdate(id:$id,input:$input){issue{id identifier state{name} labels{nodes{name}} url}}}' \
        "$(jq -n --arg id "$id" --argjson i "$input" '{id: $id, input: $i}')"
}

usage() { cat <<'EOF'
linear — Linear GraphQL wrapper (no MCP). Default team "Unbusy".

Auth: LINEAR_API_KEY in env, or a gitignored .env line in the repo root.
Get a key at https://linear.app/settings/api (Personal API keys).

Commands:
  team [--name N]                      Print the team id (verifies auth).
  list [--state S] [--label L] [--assignee me|Name]
                                       List issues (team-scoped).
  get <UNB-42|uuid>                    Full issue detail.
  comments <ref>                       Recent comments.
  create --title T [--desc D] [--labels A,B]   Create an issue.
  comment <ref> "body"                 Add a comment.
  set-labels <ref> A,B,C               REPLACE the full label set.
  add-labels <ref> A,B                 UNION labels into the current set.
  create-label <name> [--team N]       Create a label (default team Unbusy).
  update <ref> [--state S] [--labels A,B]   Set state and/or labels (closes too).
  statuses                             List workflow states for the team.
  labels                               List labels for the team.

Filters by name (state/label/assignee) resolve server-side; unknown names
error with the available values listed.
EOF
}

main() {
    local cmd="${1:-}"
    [ $# -gt 0 ] && shift || true
    case "$cmd" in
        team|create|get|list|comments|comment|set-labels|add-labels|create-label|statuses|labels|update)
            "cmd_${cmd//-/_}" "$@";;
        ""|-h|--help|help) usage;;
        *) die "unknown command: '$cmd' (see --help)";;
    esac
}

main "$@"
