"""Generate the report from measured verification evidence (requires reportlab)."""
from pathlib import Path
from html import escape
import json
import shutil

from reportlab.lib import colors
from reportlab.lib.enums import TA_CENTER
from reportlab.lib.pagesizes import A4
from reportlab.lib.styles import getSampleStyleSheet, ParagraphStyle
from reportlab.lib.units import mm
from reportlab.graphics.shapes import Drawing, Rect, Line, String, Polygon
from reportlab.platypus import SimpleDocTemplate, Paragraph, Spacer, Table, TableStyle, PageBreak

ROOT = Path(__file__).resolve().parents[1]
MODULE = ROOT / "mangahub-main" / "MangaHub-main"
EVIDENCE = MODULE / "docs" / "verification"
BLUE = colors.HexColor("#153b56")
TEAL = colors.HexColor("#087f8c")
MUTED = colors.HexColor("#526171")
WIDTH = A4[0] - 36 * mm


def architecture():
    d = Drawing(WIDTH, 280)
    def box(x, y, width, title, detail):
        d.add(Rect(x, y, width, 52, strokeColor=TEAL, fillColor=colors.HexColor("#eff8fa"), rx=5))
        d.add(String(x + width/2, y+32, title, textAnchor="middle", fontName="Helvetica-Bold", fontSize=10, fillColor=BLUE))
        d.add(String(x + width/2, y+16, detail, textAnchor="middle", fontSize=8, fillColor=MUTED))
    def arrow(x1,y1,x2,y2,label):
        d.add(Line(x1,y1,x2,y2,strokeColor=TEAL))
        d.add(Polygon([x2,y2,x2-3,y2+7,x2+3,y2+7],fillColor=TEAL,strokeColor=TEAL))
        d.add(String(x1+8,(y1+y2)/2,label,fontSize=8,fillColor=BLUE))
    box(45,220,245,"TUI clients / Go tview","Search, library, chat, alerts, connection status")
    box(45,115,245,"Gin backend / separate process","REST :8080 | TCP :9090 | UDP :9091 | WS /ws")
    box(45,10,245,"Catalog / separate process","gRPC :9092 | real Protobuf stubs")
    arrow(165,220,165,167,"HTTP + TCP + UDP + WebSocket")
    arrow(165,115,165,62,"gRPC / 2-second validation deadline")
    box(335,115,150,"Backend SQLite","Users, progress, and chat")
    box(335,10,150,"Catalog SQLite","Authoritative manga metadata")
    d.add(Line(290,141,335,141,strokeColor=TEAL))
    d.add(Line(290,36,335,36,strokeColor=TEAL))
    return d


def sequence(names, steps):
    height = 40 + len(steps)*25
    d=Drawing(WIDTH,height)
    step=WIDTH/len(names)
    for index,name in enumerate(names):
        x=step*(index+.5)
        d.add(String(x,height-13,name,textAnchor="middle",fontName="Helvetica-Bold",fontSize=8,fillColor=BLUE))
        d.add(Line(x,5,x,height-22,strokeColor=colors.HexColor("#bfccd5"),strokeDashArray=[2,3]))
    for index,(a,b,label) in enumerate(steps):
        y=height-40-index*25;x1=step*(a+.5);x2=step*(b+.5)
        d.add(Line(x1,y,x2,y,strokeColor=TEAL))
        sign=1 if x2>x1 else -1
        d.add(Polygon([x2,y,x2-sign*6,y+3,x2-sign*6,y-3],fillColor=TEAL,strokeColor=TEAL))
        d.add(String((x1+x2)/2,y+5,label,textAnchor="middle",fontSize=7,fillColor=MUTED))
    return d


def generate_report():
    record=json.loads((EVIDENCE/"connected-review-2026-11-20.json").read_text(encoding="utf-8-sig"))
    if record["failed"] or not record["checks"]:
        raise RuntimeError("The connected verifier must pass before regenerating this report")
    acceptance = json.loads((EVIDENCE / "postman-feature-acceptance.json").read_text(encoding="utf-8-sig"))
    if not acceptance["passed"]:
        raise RuntimeError("The acceptance verifier must pass before regenerating this report")
    request_count = sum(phase["stats"]["requests"]["total"] for phase in acceptance["postman_phases"])
    assertion_count = sum(phase["stats"]["assertions"]["total"] for phase in acceptance["postman_phases"])
    styles=getSampleStyleSheet()
    styles.add(ParagraphStyle(name="ReportBody",fontName="Helvetica",fontSize=9.5,leading=13,spaceAfter=8,textColor=colors.HexColor("#223342")))
    styles.add(ParagraphStyle(name="SmallCell",fontSize=8,leading=10,textColor=colors.HexColor("#223342")))
    styles.add(ParagraphStyle(name="ReportTitle",fontName="Helvetica-Bold",fontSize=27,leading=33,textColor=BLUE,spaceAfter=18))
    styles["Heading1"].textColor=BLUE;styles["Heading1"].fontSize=19;styles["Heading1"].leading=23
    styles["Heading2"].textColor=TEAL;styles["Heading2"].fontSize=12;styles["Heading2"].leading=15
    story=[];md=["# MangaHub Technical Reasoning and Verification Report\n"]
    def para(text):
        story.append(Paragraph(escape(text),styles["ReportBody"]));md.append(text+"\n")
    def heading(text):
        story.append(Paragraph(escape(text),styles["Heading1"]));md.append("\n## "+text+"\n")
    def sub(text):
        story.append(Paragraph(escape(text),styles["Heading2"]));md.append("\n### "+text+"\n")
    def table(headers,rows,widths):
        data=[[Paragraph(escape(str(x)),styles["SmallCell"]) for x in row] for row in [headers]+rows]
        t=Table(data,colWidths=widths,repeatRows=1,hAlign="LEFT")
        t.setStyle(TableStyle([("BACKGROUND",(0,0),(-1,0),colors.HexColor("#dceef1")),("VALIGN",(0,0),(-1,-1),"TOP"),("BOX",(0,0),(-1,-1),.5,colors.HexColor("#b6c9d0")),("INNERGRID",(0,0),(-1,-1),.3,colors.HexColor("#d4dfe3")),("LEFTPADDING",(0,0),(-1,-1),6),("RIGHTPADDING",(0,0),(-1,-1),6),("TOPPADDING",(0,0),(-1,-1),3),("BOTTOMPADDING",(0,0),(-1,-1),3)]))
        story.append(t);story.append(Spacer(1,10))
        md.append("| "+" | ".join(headers)+" |\n| "+" | ".join(["---"]*len(headers))+" |")
        md.extend("| "+" | ".join(str(x).replace("|","/") for x in row)+" |" for row in rows);md.append("")
    def nextpage():story.append(PageBreak())
    story.append(Spacer(1,38*mm));story.append(Paragraph("MangaHub",styles["ReportTitle"]))
    story.append(Paragraph("Technical reasoning<br/>and verification report",styles["ReportTitle"]))
    para("IT096IIU - Net-Centric Programming | International University, VNU-HCM")
    para("Student specification v3.0 | 23 November 2026")
    table(["Team member ID","Contribution record"],[["ITITWE23049","To be confirmed by this member"],["ITITIU22130","To be confirmed by this member"]],[150,WIDTH-150])
    para(f"The connected verifier passed {record['passed']}/{len(record['checks'])} checks. The acceptance suite passed {request_count} HTTP requests, {assertion_count} assertions, and live TUI, socket and database checks.")
    para("Implementation and verification are recorded below. Individual contribution allocation and the live ownership demonstration remain to be confirmed by the two members.")
    nextpage()
    heading("1. System architecture")
    para("MangaHub tracks metadata and reading progress. It does not host manga reading pages. Three independently launched Go programs separate the user interface, the Gin gateway and the catalog service.")
    story.append(architecture());md.append("The TUI connects to Gin over HTTP, TCP, UDP and WebSocket. Gin calls the separate gRPC catalog. Backend SQLite owns users, progress and chat. Catalog SQLite owns authoritative manga metadata.\n")
    sub("Data and process ownership")
    para("cmd/catalog owns the authoritative catalog and seeds 54 titles from data/manga_seed.json when empty. cmd/backend stores users, user_progress and chat_messages, and enforces JWT identity. Catalog validation, search by title or author, and manga details go through gRPC.")
    para("The TUI performs HTTP transactions and runs TCP, UDP and WebSocket workers. Network callbacks dispatch widget changes through tview's event loop. Shared connection and authentication state uses a mutex. SQLite writes are serialized through one backend connection; an UPDATE ... RETURNING statement binds every response version to its own committed update.")
    sub("Configuration and trust boundaries")
    para("Ports and database paths are supplied through environment variables. The backend loads JWT_SECRET at startup. Chapter release additionally requires X-Admin-Key matching ADMIN_API_KEY; an empty key disables release. TCP and WebSocket identity comes from verified JWT claims, never a supplied username or user ID. This localhost teaching setup does not provide TLS for remote deployment.")
    nextpage()
    heading("2. Why these technologies?")
    protocols=[
      ("HTTP REST","Feature: authentication, search, library and progress. Data flow: TUI JSON request -> Gin -> SQLite or gRPC. Why: explicit methods, statuses and request/response semantics. Failure: invalid credentials return 401; invalid input 400; catalog transport failure 503. The TUI uses a five-second HTTP timeout. Alternative: RPC-only clients would lose the simple inspectable REST contract."),
      ("TCP","Feature: push progress to another session of the same account. Flow: committed update -> user-scoped Hub -> newline JSON frame -> peer TUI. Why: ordered reliable byte transport with long-lived connections. A stream needs an explicit newline delimiter. Failure: authentication and socket writes have deadlines; disconnect unregisters the client; slow buffers are dropped; the client retries. Reliability does not imply an offline event history."),
      ("UDP","Chapter releases send one JSON datagram per subscribed return address. UDP offers lightweight fan-out for advisory alerts. The TUI waits for a subscription acknowledgement and renews it periodically. UDP packets can be lost. If the server cannot bind its UDP port, chapter releases still persist and report alerts as unavailable."),
      ("WebSocket","An authenticated socket sends chat in both directions without polling. The backend assigns the author and room, commits the message to SQLite, then broadcasts it. The TUI keeps the draft if the write fails or no echo arrives. Joining a room replays its latest 50 messages after reconnect or restart."),
      ("gRPC","Feature: the independent catalog boundary. Flow: Gin -> generated Protobuf client -> catalog SQLite -> typed response. Why: an explicit typed service contract and real service-to-service calls. Failure: NotFound maps to 404, invalid requests to 400, and unavailable/deadline errors to 503. Library writes stop before mutation when catalog validation fails. REST between these services would not demonstrate the required gRPC boundary.")]
    for title,text in protocols:sub(title);para(text)
    para("Redis is not implemented. Under v3.0 it is optional; the verification effort focuses on identity, isolation, concurrency and controlled failures.")
    nextpage()
    heading("3. Important end-to-end flows")
    sub("Reading progress and same-user synchronization")
    story.append(sequence(["TUI A","Gin backend","Catalog","Backend DB","TUI B"],[(0,1,"PUT progress / JWT"),(1,2,"GetManga / 2s deadline"),(2,1,"Authoritative chapter limit"),(1,3,"Validate; UPDATE RETURNING version"),(3,1,"Committed chapter + version"),(1,4,"TCP newline JSON / same user only")]))
    para("Gin validates a non-negative chapter without narrowing it to int32, checks it against the catalog total and validates the reading status. One atomic statement updates the stored chapter/status/version. The Hub publishes only after success. A peer renders the change without refresh; stale or duplicate event versions cannot roll the displayed progress back. An unavailable catalog returns an error and leaves the stored version and chapter unchanged.")
    sub("Protected release and best-effort notifications")
    story.append(sequence(["Admin TUI","Gin backend","Catalog DB","Subscribers"],[(0,1,"POST release / JWT + admin key"),(1,2,"UpdateChapters over gRPC"),(2,1,"Successful catalog update"),(1,3,"UDP new_chapter datagrams")]))
    para("The admin route requires JWT and the admin key. An atomic catalog update accepts only a chapter strictly above the current total. A repeated or lower chapter returns HTTP 409 before UDP fan-out. After a successful write, the backend sends the event to registered addresses if UDP is available. Delivery remains best effort.")
    nextpage()
    heading("4. Verification and observed results")
    para("The retained connected verifier builds independent catalog/backend processes with temporary databases, ports, accounts and keys. It exercises real sockets and launches three actual tview applications using tcell simulation screens. It does not claim that two students have completed the human live demonstration.")
    rows=[]
    descriptions={"Live TUI integration under race detector":"Peer progress, UDP alerts/unsubscribe, chat/isolation, disconnect and navigation rendered in three TUIs.","Expired JWT rejected across protocols":"REST, TCP and WebSocket rejected the same expired signed token.","Concurrent progress commits":"Eight concurrent HTTP updates had distinct versions and a matching final stored chapter."}
    for item in record["checks"]:
      observed=descriptions.get(item["check"],item["observed"])
      if len(observed)>105:observed=observed[:102]+"..."
      rows.append([item["check"],"PASS",observed])
    table(["Connected scenario","Result","Observed"],rows,[168,38,WIDTH-206])
    para("Reproduce from the repository root: python scripts/verify_connected.py --with-tui. Detailed results and the embedded live TUI test output are in docs/verification/connected-review-2026-11-20.json. The verifier exits nonzero on failure and does not modify the user's saved session or data.")
    para("From the Go module: go test ./... -count=1; go test -race ./... -count=1; go vet ./.... The unit suite adds zero/negative/overflow chapters, missing catalog responses, error mapping on both route aliases, token algorithm/expiry/identity checks and twelve concurrent progress updates. Retained Go results are in docs/verification/go-tests.jsonl and race-tests.txt.")
    nextpage()
    heading("5. Reflection and ownership")
    sub("A real defect: identity validation and fail-open writes")
    para("Symptom: the initial connected review passed 9/12 checks, but a tokenless TCP user_id was accepted, WebSocket upgraded without JWT, and an above-total progress update returned HTTP 200 after the catalog process stopped. Cause: identity fallbacks bypassed JWT, the WebSocket route lacked authentication, and the library handler ignored gRPC errors before writing SQLite.")
    para("Fix: remove TCP identity fallbacks; share strict HS256/expiry/identity validation across REST and sockets; derive WebSocket author/room on the server; centralize mandatory catalog validation before both library write paths. Retest: the original triggers now fail safely while valid same-user synchronization, room chat, UDP release and restart persistence continue to pass. The table on the previous page records the current measured run.")
    sub("Known limitations and design tradeoffs")
    para("Progress uses the latest committed write and clients discard stale versions. Refresh the library after an offline period to reconcile missed TCP updates. SQLite serializes writes. Room membership and UDP alert feeds are in memory, and chat messages persist in SQLite. UDP can lose packets. The server validates tokens when a socket connects. The demo runs on localhost, and the project uses no Redis.")
    para("The acceptance suite verifies that a missing or null chapter leaves progress and version intact, while chapter zero remains valid. It also checks concurrent duplicate releases, UDP port conflicts and SQLite chat replay after both processes restart. See docs/verification/postman-feature-acceptance.json.")
    sub("Member contributions and live ownership")
    table(["Member ID","Contribution record"],[["ITITWE23049","Member must confirm the components and tests they implemented or reviewed."],["ITITIU22130","Member must confirm the components and tests they implemented or reviewed."]],[130,WIDTH-130])
    para("Both members should be able to explain one complete request path, the TCP framing boundary, the UDP delivery limitation, room isolation, and one failure/debug case without AI assistance. The report does not invent a contribution split.")
    sub("Three recorded AI-assisted interactions")
    para("1. Specification review: AI compared v3.0 with code, distinguished the older points rubric from the current letter-grade criteria, and created a connected verifier. 2. Review decision: an initially passing unit suite was not accepted as proof of A readiness; real-process tests reproduced three missing boundaries and drove the fixes. 3. Implementation and verification: AI assisted the TUI redesign, auth/failure fixes and regression tests; actual three-TUI race-enabled execution verified peer progress, UDP unsubscribe, authenticated chat and disconnect behavior.")
    para("Reference: IT096IU MangaHub Project v3.0, sections 3-8; committed .proto, source/tests and retained verification outputs. Final assessment remains based on the live application, this evidence and both students' explanations.")
    def footer(canvas,doc):
      canvas.setStrokeColor(colors.HexColor("#cfdae0"));canvas.line(18*mm,16*mm,A4[0]-18*mm,16*mm)
      canvas.setFont("Helvetica",8);canvas.setFillColor(MUTED)
      canvas.drawString(18*mm,11*mm,"MangaHub | IT096IIU | 23 November 2026")
      canvas.drawRightString(A4[0]-18*mm,11*mm,str(doc.page))
    output=ROOT/"REPORT.pdf"
    SimpleDocTemplate(str(output),pagesize=A4,rightMargin=18*mm,leftMargin=18*mm,topMargin=18*mm,bottomMargin=22*mm,title="MangaHub Technical Reasoning and Verification",author="ITITWE23049; ITITIU22130").build(story,onFirstPage=footer,onLaterPages=footer)
    markdown="\n".join(md).rstrip()+"\n"
    for path in [ROOT/"REPORT.md",MODULE/"REPORT.md"]:path.write_text(markdown,encoding="utf-8",newline="\n")
    for path in [MODULE/"REPORT.pdf",MODULE/"docs"/"REPORT.pdf"]:shutil.copyfile(output,path)
    print("Generated and synchronized REPORT.pdf and REPORT.md from measured evidence")


if __name__ == "__main__":
    generate_report()
