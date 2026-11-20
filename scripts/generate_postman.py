"""Build the importable Postman v2.1 collection and a secret-free local environment."""
import json
from pathlib import Path
import textwrap

ROOT = Path(__file__).resolve().parents[1]
DOCS = ROOT / "mangahub-main" / "MangaHub-main" / "docs"


def event(kind, code):
    # The sandbox exposes legacy globals such as `data`; isolate local bindings.
    lines = ["(() => {"] + textwrap.dedent(code).strip().splitlines() + ["})();"]
    return {"listen": kind, "script": {"type": "text/javascript", "exec": lines}}


def request(name, method, path, status=200, body=None, tests="", pre="", auth=None, headers=None, description=""):
    req = {"method": method, "header": [{"key": "Accept", "value": "application/json"}], "url": "{{baseUrl}}" + path,
           "description": description}
    if body is not None:
        req["header"].append({"key": "Content-Type", "value": "application/json"})
        req["body"] = {"mode": "raw", "raw": body if isinstance(body, str) else json.dumps(body, indent=2), "options": {"raw": {"language": "json"}}}
    if auth == "none": req["auth"] = {"type": "noauth"}
    elif auth: req["auth"] = {"type": "bearer", "bearer": [{"key": "token", "value": auth, "type": "string"}]}
    req["header"].extend({"key": key, "value": value} for key, value in (headers or {}).items())
    script = f'pm.test("HTTP {status}", () => pm.response.to.have.status({status}));\n'
    if status >= 400: script += 'pm.test("Structured error", () => pm.expect(pm.response.json().error).to.be.a("string").and.not.empty);\n'
    script += textwrap.dedent(tests).strip()
    item = {"name": name, "request": req, "event": [event("test", script)], "response": []}
    if pre: item["event"].insert(0, event("prerequest", pre))
    return item


def folder(name, items, description="", flag=None):
    result = {"name": name, "description": description, "item": items}
    if flag:
        result["event"] = [event("prerequest", f'''
            if (String(pm.variables.get("{flag}")) !== "true") {{ pm.execution.skipRequest(); }}
            {'if (!pm.variables.get("adminKey")) { throw new Error("Set adminKey before running the admin demo"); }' if flag == 'runAdminDemo' else ''}
        ''')]
    return result


STORE = 'const save = (key, value) => { pm.collectionVariables.set(key, value); pm.environment.set(key, value); };\n'
SAVE_AUTH = STORE + '''
const data = pm.response.json();
pm.test("JWT and verified identity returned", () => {
  pm.expect(data.token).to.be.a("string").and.not.empty;
  pm.expect(data.user_id).to.be.a("string").and.not.empty;
  pm.expect(data.username).to.eql(pm.variables.get("testUsername"));
});
if (pm.response.code === 200) { save("token",data.token); save("userId",data.user_id); save("username",data.username); }
'''
REGISTER_PRE = STORE + '''
const suffix = pm.variables.replaceIn("{{$guid}}").replace(/-/g, "");
save("testUsername", "pm_" + suffix);
save("testEmail", "pm_" + suffix + "@example.test");
save("password", "MangaHub_demo_" + suffix + "!");
save("token", ""); save("userId", "");
'''
LIBRARY_CHECK = '''
const body = pm.response.json();
pm.test("Only this account's library entry", () => {
  pm.expect(body.count).to.eql(1);
  const item = body.library.find(x => x.manga_id === pm.variables.get("mangaId"));
  pm.expect(item).to.be.an("object");
  pm.expect(item.current_chapter).to.eql(Number(pm.variables.get("expectedChapter")));
  pm.expect(item.version).to.eql(Number(pm.variables.get("lastVersion")));
});
'''
SAVE_PROGRESS = STORE + '''
const data = pm.response.json();
pm.test("Exact next version and saved chapter", () => {
  pm.expect(data.manga_id).to.eql(pm.variables.get("mangaId"));
  pm.expect(data.version).to.eql(Number(pm.variables.get("lastVersion")) + 1);
  pm.expect(data.chapter).to.eql(Number(pm.variables.get("targetChapter")));
});
if (pm.response.code === 200) { save("lastVersion",data.version); save("expectedChapter",data.chapter); }
'''


def build():
    auth_body = {"username": "{{testUsername}}", "password": "{{password}}"}
    register_body = dict(auth_body, email="{{testEmail}}")
    progress = '/api/library/{{mangaId}}/progress'
    admin = '/api/admin/manga/{{mangaId}}/release'
    empty_test = 'pm.test("Empty personal library", () => { const d=pm.response.json(); pm.expect(d.library).to.eql([]); pm.expect(d.count).to.eql(0); });'
    catalogue_test = 'pm.test("Catalog response", () => { const d=pm.response.json(); pm.expect(d.manga).to.be.an("array"); pm.expect(d.count).to.eql(d.manga.length); });'
    items = [folder("01 - Health", [
        request("Backend and catalog ready", "GET", "/api/health", auth="none", tests='pm.test("Catalog connected", () => pm.expect(pm.response.json().services.grpc_catalog).to.eql("connected"));', description="This proves HTTP and the catalog RPC respond. It does not prove TCP/UDP client delivery.")
    ]), folder("02 - F01 Accounts", [
        request("Register a fresh demo account", "POST", "/api/auth/register", body=register_body, auth="none", pre=REGISTER_PRE, tests=SAVE_AUTH),
        request("Duplicate username is rejected", "POST", "/api/auth/register", 409, body=register_body, auth="none"),
        request("Short password is rejected", "POST", "/api/auth/register", 400, body={"username":"invalid_demo","email":"invalid@example.test","password":"short"}, auth="none"),
        request("Successful login and save JWT", "POST", "/api/auth/login", body=auth_body, auth="none", tests=SAVE_AUTH),
        request("Wrong password is rejected", "POST", "/api/auth/login", 401, body={"username":"{{testUsername}}","password":"deliberately-wrong-password"}, auth="none"),
        request("Protected library without token", "GET", "/api/library", 401, auth="none"),
        request("Protected catalog with invalid JWT", "GET", "/api/manga", 401, auth="not-a-valid-jwt")
    ], "Registration generates fresh credentials on each run. JWT values stay in the active local environment; do not share an environment containing live credentials."),
    folder("03 - F02 F06 Catalog", [
        request("List catalog and select real manga IDs", "GET", "/api/manga", tests=STORE+catalogue_test+'''
            const list=pm.response.json().manga;
            pm.test("At least 30 seeded titles", () => pm.expect(list.length).to.be.at.least(30));
            if (list.length >= 2) {
              const m=list[0]; save("mangaId",m.id); save("otherMangaId",list[1].id);
              save("mangaTitle",m.title); save("searchQuery",m.title.split(" ")[0]);
              save("genre",m.genres[0]); save("totalChapters",Number(m.total_chapters||0));
              save("overflowChapter",Number(m.total_chapters||0)+1);
            }
        '''),
        request("Search by title", "GET", "/api/manga?q={{searchQuery}}", tests=catalogue_test+'pm.test("Selected title found", () => pm.expect(pm.response.json().manga.some(m=>m.id===pm.variables.get("mangaId"))).to.eql(true));'),
        request("Filter by genre", "GET", "/api/manga?genre={{genre}}", tests=catalogue_test+'pm.test("Genre filter applied", () => { pm.expect(pm.response.json().count).to.be.above(0); pm.response.json().manga.forEach(m=>pm.expect(m.genres.join(",").toLowerCase()).to.include(String(pm.variables.get("genre")).toLowerCase())); });'),
        request("No search results", "GET", "/api/manga?q=__postman_no_matching_title_72819__", tests='pm.test("No matches", () => {const d=pm.response.json(); pm.expect(d.count).to.eql(0); pm.expect(d.manga || []).to.eql([]);});'),
        request("Details through Gin to gRPC", "GET", "/api/manga/{{mangaId}}", tests='pm.test("All required detail fields", () => { const d=pm.response.json(); ["id","title","author","genres","status","total_chapters","description"].forEach(k=>pm.expect(d).to.have.property(k)); pm.expect(d.id).to.eql(pm.variables.get("mangaId")); });'),
        request("Unknown catalog ID", "GET", "/api/manga/__postman_missing_id__", 404)
    ], "Gin calls the separately running gRPC catalog; these are HTTP requests exercising that boundary indirectly."),
    folder("04 - F03 Library and progress", [
        request("New account has an empty library", "GET", "/api/library", tests=empty_test),
        request("Add as plan-to-read", "POST", "/api/library", body={"manga_id":"{{mangaId}}","status":"plan_to_read"}),
        request("Repeated add does not duplicate entry", "POST", "/api/library", body={"manga_id":"{{mangaId}}","status":"plan_to_read"}),
        request("Verify persisted initial state", "GET", "/api/library", tests=STORE+'''
            const d=pm.response.json();
            pm.test("One plan-to-read entry at chapter zero", () => {pm.expect(d.count).to.eql(1); pm.expect(d.library[0].current_chapter).to.eql(0); pm.expect(d.library[0].status).to.eql("plan_to_read");});
            save("lastVersion",d.library[0].version); save("expectedChapter",0);
        '''),
        request("Set reading progress", "PUT", progress, body='{"current_chapter":{{targetChapter}},"status":"reading"}', pre=STORE+'save("targetChapter", Math.min(3,Number(pm.variables.get("totalChapters"))));', tests=SAVE_PROGRESS),
        request("Chapter zero is valid", "PUT", progress, body={"current_chapter":0,"status":"reading"}, pre=STORE+'save("targetChapter",0);', tests=SAVE_PROGRESS),
        request("Mark completed at total chapters", "PUT", progress, body='{"current_chapter":{{targetChapter}},"status":"completed"}', pre=STORE+'save("targetChapter",Number(pm.variables.get("totalChapters")));', tests=SAVE_PROGRESS),
        request("Verify stored chapter and version", "GET", "/api/library", tests=LIBRARY_CHECK),
        request("Reject negative chapter", "PUT", progress, 400, body={"current_chapter":-1}),
        request("Reject chapter above total", "PUT", progress, 400, body='{"current_chapter":{{overflowChapter}}}'),
        request("Reject int32 overflow input", "PUT", progress, 400, body={"current_chapter":4294967296}),
        request("Reject unknown reading status", "PUT", progress, 400, body={"current_chapter":0,"status":"invented"}),
        request("Existing manga not in this library", "PUT", "/api/library/{{otherMangaId}}/progress", 404, body={"current_chapter":0}),
        request("Rejected writes left data unchanged", "GET", "/api/library", tests=LIBRARY_CHECK)
    ], "These requests verify HTTP persistence. Observe a second TUI of the same account to prove visible TCP delivery; HTTP 200 alone is not delivery proof."),
    folder("05 - F01 User isolation and admin guards", [
        request("Register a separate Bob account", "POST", "/api/auth/register", body={"username":"{{bobUsername}}","email":"{{bobEmail}}","password":"{{bobPassword}}"}, auth="none", pre=STORE+'''
            const id=pm.variables.replaceIn("{{$guid}}").replace(/-/g,"");
            save("bobUsername","bob_"+id);save("bobEmail","bob_"+id+"@example.test");save("bobPassword","Bob_demo_"+id+"!");
        ''', tests=STORE+'const d=pm.response.json(); if(pm.response.code===200){save("bobToken",d.token);save("bobUserId",d.user_id);} pm.test("Different account",()=>pm.expect(d.user_id).not.to.eql(pm.variables.get("userId")));'),
        request("Bob cannot read Alice's library", "GET", "/api/library?user_id={{userId}}", auth="{{bobToken}}", tests=empty_test),
        request("Bob cannot edit Alice's library", "PUT", progress, 404, body={"current_chapter":0,"user_id":"{{userId}}"}, auth="{{bobToken}}"),
        request("Alice's stored state remains unchanged", "GET", "/api/library", tests=LIBRARY_CHECK),
        request("Release requires JWT", "POST", admin, 401, body={"chapter":1}, auth="none"),
        request("Release requires admin key", "POST", admin, 403, body={"chapter":1}),
        request("Wrong admin key is rejected", "POST", admin, 403, body={"chapter":1}, headers={"X-Admin-Key":"deliberately-wrong-key"})
    ]),
    folder("06 - F04 Admin demo (opt-in)", [
        request("Read current authoritative chapter count", "GET", "/api/manga/{{mangaId}}", tests=STORE+'save("newChapter",Number(pm.response.json().total_chapters||0)+1);'),
        request("Publish one real chapter and broadcast UDP", "POST", admin, body='{"chapter":{{newChapter}}}', headers={"X-Admin-Key":"{{adminKey}}"}, tests='pm.test("Catalog update acknowledged",()=>pm.expect(pm.response.json().total_chapters).to.eql(Number(pm.variables.get("newChapter"))));', description="Changes catalog data. Open subscribed TUI clients to observe the actual UDP notification."),
        request("Verify changed catalog count", "GET", "/api/manga/{{mangaId}}", tests=STORE+'pm.test("New total persisted",()=>pm.expect(pm.response.json().total_chapters).to.eql(Number(pm.variables.get("newChapter"))));save("totalChapters",pm.response.json().total_chapters);'),
        request("Reject invalid release chapter", "POST", admin, 400, body={"chapter":0}, headers={"X-Admin-Key":"{{adminKey}}"}),
        request("Reject overflowing release chapter", "POST", admin, 400, body={"chapter":4294967296}, headers={"X-Admin-Key":"{{adminKey}}"}),
        request("Unknown release manga", "POST", "/api/admin/manga/__postman_missing_id__/release", 404, body={"chapter":1}, headers={"X-Admin-Key":"{{adminKey}}"})
    ], "Default skipped. Set runAdminDemo=true and adminKey to the backend ADMIN_API_KEY. Uses a real release; use a disposable demo database.", flag="runAdminDemo"),
    folder("07 - Legacy route aliases", [
        request("Legacy login", "POST", "/auth/login", body=auth_body, auth="none", tests=SAVE_AUTH),
        request("Legacy catalog search", "GET", "/manga?q={{searchQuery}}", tests=catalogue_test),
        request("Legacy manga details", "GET", "/manga/{{mangaId}}"),
        request("Legacy library", "GET", "/users/library", tests=LIBRARY_CHECK),
        request("Legacy progress update", "PUT", "/users/progress", body='{"manga_id":"{{mangaId}}","current_chapter":{{targetChapter}},"status":"reading"}', pre=STORE+'save("targetChapter",Math.min(5,Number(pm.variables.get("totalChapters"))));', tests=SAVE_PROGRESS),
        request("Verify legacy update through current route", "GET", "/api/library", tests=LIBRARY_CHECK)
    ]),
    folder("08 - F06 Catalog offline (opt-in)", [
        request("Backend alive but catalog unreachable", "GET", "/api/health", auth="none", tests='pm.test("Catalog is down",()=>pm.expect(pm.response.json().services.grpc_catalog).to.eql("unreachable"));'),
        request("Search fails with bounded 503", "GET", "/api/manga?q={{searchQuery}}", 503, tests='pm.test("Bounded response",()=>pm.expect(pm.response.responseTime).to.be.below(5000));'),
        request("Details fail with 503", "GET", "/api/manga/{{mangaId}}", 503),
        request("Add fails with 503", "POST", "/api/library", 503, body={"manga_id":"{{otherMangaId}}"}),
        request("Progress fails with 503", "PUT", progress, 503, body={"current_chapter":0}),
        request("Stored chapter and version did not change", "GET", "/api/library", tests=LIBRARY_CHECK)
    ], "Run the normal workflow first, stop only catalog, set runFaultChecks=true, then select only this folder in Runner. Restart catalog afterward.", flag="runFaultChecks"),
    folder("09 - Restart and recovery (opt-in)", [
        request("Catalog has recovered", "GET", "/api/health", auth="none", tests='pm.test("Catalog connected",()=>pm.expect(pm.response.json().services.grpc_catalog).to.eql("connected"));'),
        request("Existing account logs in after restart", "POST", "/api/auth/login", body=auth_body, auth="none", tests=SAVE_AUTH),
        request("Library survives backend restart", "GET", "/api/library", tests=LIBRARY_CHECK),
        request("Details available again", "GET", "/api/manga/{{mangaId}}")
    ], "Restart backend/catalog with the same database paths and JWT_SECRET. Set runRecoveryChecks=true and run only this folder.", flag="runRecoveryChecks")]
    defaults={"baseUrl":"http://localhost:8080","wsUrl":"ws://localhost:8080/ws","grpcAddress":"localhost:9092","tcpAddress":"localhost:9090","udpAddress":"localhost:9091","adminKey":"","runAdminDemo":"false","runFaultChecks":"false","runRecoveryChecks":"false","wsRoom":"general"}
    runtime=["token","userId","username","testUsername","testEmail","password","mangaId","otherMangaId","mangaTitle","searchQuery","genre","totalChapters","overflowChapter","targetChapter","expectedChapter","lastVersion","newChapter","bobToken","bobUserId","bobUsername","bobEmail","bobPassword"]
    values=dict(defaults,**{key:"" for key in runtime})
    collection={"info":{"_postman_id":"d8f4b3e1-9c2a-4f10-8a7e-5b3c1d2e4f60","name":"MangaHub - F01-F06 Acceptance API","schema":"https://schema.getpostman.com/json/collection/v2.1.0/collection.json","description":"Run folders 01-05 and 07 in order with both services running. Fresh demo users, JWT and real manga IDs are saved automatically. Admin release, catalog outage and restart folders are opt-in. HTTP assertions are not proof of TCP/UDP/TUI delivery. See POSTMAN_GUIDE.md for live TUI, WebSocket and gRPC checks."},"auth":{"type":"bearer","bearer":[{"key":"token","value":"{{token}}","type":"string"}]},"variable":[{"key":key,"value":value,"type":"string"} for key,value in values.items()],"item":items}
    secrets={"token","bobToken","password","bobPassword","adminKey"}
    environment={"id":"4cb88996-7531-44e0-8e70-145dba6038aa","name":"MangaHub Local (demo)","values":[{"key":key,"value":value,"type":"secret" if key in secrets else "default","enabled":True} for key,value in values.items()],"_postman_variable_scope":"environment"}
    for filename,data in [("MangaHub_API.postman_collection.json",collection),("MangaHub_Local.postman_environment.json",environment)]:
        (DOCS/filename).write_text(json.dumps(data,indent=2)+"\n",encoding="utf-8",newline="\n")
    print(f"Generated {sum(len(x['item']) for x in items)} requests in {len(items)} folders and a secret-free environment")


if __name__ == "__main__":
    build()
