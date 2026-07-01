import AppKit
import ServiceManagement
import UserNotifications
import WebKit
func tlog(_ msg: String) {
    NSLog("[Yucca] %@", msg)
}

// Tray mark — single-colour (template) yucca bloom.
let yuccaIdleSVG = """
<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 120 120"><circle cx="60" cy="60" r="42" fill="none" stroke="black" stroke-width="8"/><g transform="translate(60 60) scale(1.0) translate(-61 -65)"><g transform="translate(60 106) rotate(-30)"><path fill="black" d="M -7 0 C -6.4 -25.2, -2.9 -48, 0 -60 C 2.9 -48, 6.4 -25.2, 7 0 Z"/></g><g transform="translate(60 106) rotate(-6)"><path fill="black" d="M -8 0 C -7.4 -34.4, -3.4 -65.6, 0 -82 C 3.4 -65.6, 7.4 -34.4, 8 0 Z"/></g><g transform="translate(60 106) rotate(15)"><path fill="black" d="M -7.4 0 C -6.8 -30.2, -3.1 -57.6, 0 -72 C 3.1 -57.6, 6.8 -30.2, 7.4 0 Z"/></g><g transform="translate(60 106) rotate(39)"><path fill="black" d="M -6 0 C -5.5 -21.8, -2.5 -41.6, 0 -52 C 2.5 -41.6, 5.5 -21.8, 6 0 Z"/></g></g></svg>
"""

// Shown when an approval is waiting — the ring fills in to signal action required.
let yuccaActionSVG = """
<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 120 120"><circle cx="60" cy="60" r="42" fill="none" stroke="black" stroke-width="8"/><circle cx="60" cy="60" r="26" fill="black"/></svg>
"""

let yuccaOfflineSVG = """
<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 120 120" opacity="0.4"><circle cx="60" cy="60" r="42" fill="none" stroke="black" stroke-width="8"/><g transform="translate(60 60) scale(1.0) translate(-61 -65)"><g transform="translate(60 106) rotate(-30)"><path fill="black" d="M -7 0 C -6.4 -25.2, -2.9 -48, 0 -60 C 2.9 -48, 6.4 -25.2, 7 0 Z"/></g><g transform="translate(60 106) rotate(-6)"><path fill="black" d="M -8 0 C -7.4 -34.4, -3.4 -65.6, 0 -82 C 3.4 -65.6, 7.4 -34.4, 8 0 Z"/></g><g transform="translate(60 106) rotate(15)"><path fill="black" d="M -7.4 0 C -6.8 -30.2, -3.1 -57.6, 0 -72 C 3.1 -57.6, 6.8 -30.2, 7.4 0 Z"/></g><g transform="translate(60 106) rotate(39)"><path fill="black" d="M -6 0 C -5.5 -21.8, -2.5 -41.6, 0 -52 C 2.5 -41.6, 5.5 -21.8, 6 0 Z"/></g></g></svg>
"""

// Lucide icons (https://lucide.dev, ISC) for the credential menu.
let folderSVG = """
<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="black" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/></svg>
"""

// Active project: green open folder (rendered in color, not as a template).
let folderActiveSVG = """
<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="#34C759" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="m6 14 1.5-2.9A2 2 0 0 1 9.24 10H20a2 2 0 0 1 1.94 2.5l-1.54 6a2 2 0 0 1-1.95 1.5H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h3.9a2 2 0 0 1 1.69.9l.81 1.2a2 2 0 0 0 1.67.9H18a2 2 0 0 1 2 2v2"/></svg>
"""

let keySVG = """
<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="black" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m15.5 7.5 2.3 2.3a1 1 0 0 0 1.4 0l2.1-2.1a1 1 0 0 0 0-1.4L21 4"/><path d="m21 2-9.6 9.6"/><circle cx="7.5" cy="15.5" r="5.5"/></svg>
"""

func svgToImage(_ svg: String, fallbackSymbol: String) -> NSImage {
    if let data = svg.data(using: .utf8), let image = NSImage(data: data) {
        image.isTemplate = true
        image.size = NSSize(width: 18, height: 18)
        return image
    }
    tlog("SVG failed, trying SF Symbol: \(fallbackSymbol)")
    if let symbol = NSImage(systemSymbolName: fallbackSymbol, accessibilityDescription: "Yucca") {
        return symbol
    }
    tlog("SF Symbol failed too, using caution icon")
    return NSImage(systemSymbolName: "shield", accessibilityDescription: "Yucca")
        ?? NSImage(named: NSImage.cautionName)!
}

// menuIcon renders a Lucide SVG as a small menu-item image. template:true
// (default) makes it monochrome and adapt to light/dark; template:false keeps
// the SVG's own colors (e.g. the green "active" folder).
func menuIcon(_ svg: String, fallback: String, template: Bool = true) -> NSImage {
    let img = svgToImage(svg, fallbackSymbol: fallback)
    img.size = NSSize(width: 15, height: 15)
    img.isTemplate = template
    return img
}

struct RequestData {
    let id: String
    let kind: String          // "execute_accept" or "secret_request"
    let alias: String         // set for secret_request
    let aliases: [String]     // set for execute_accept
    let reason: String
    let projectPath: String
    let projectName: String
    let projectSlug: String
    let status: String
    let createdAt: String
}

struct WSMessage {
    let type: String
    let project: String?
    let data: [String: Any]?

    init?(from jsonData: Data) {
        guard let json = try? JSONSerialization.jsonObject(with: jsonData) as? [String: Any],
              let type = json["type"] as? String else { return nil }
        self.type = type
        self.project = json["project"] as? String
        self.data = json["data"] as? [String: Any]
    }
}

struct DaemonInfo: Codable {
    let pid: Int
    let port: Int
    let addr: String
    let started_at: String
}

func discoverDaemon() -> DaemonInfo? {
    let path = NSString(string: "~/.yucca/daemon.json").expandingTildeInPath
    guard let data = FileManager.default.contents(atPath: path),
          let info = try? JSONDecoder().decode(DaemonInfo.self, from: data) else {
        return nil
    }
    if kill(Int32(info.pid), 0) != 0 { return nil }
    return info
}

class AppDelegate: NSObject, NSApplicationDelegate, UNUserNotificationCenterDelegate {
    var statusItem: NSStatusItem!
    var daemon: DaemonInfo?
    var healthTimer: Timer?
    var connected = false
    var wsTask: URLSessionWebSocketTask?
    var pendingRequests: [String: RequestData] = [:]
    var orderedProjects: [(slug: String, name: String, active: Bool)] = []  // active first
    var projectCredentials: [String: [String]] = [:]  // slug -> sorted aliases
    var projectNotes: [String: [(alias: String, body: String)]] = [:]  // slug -> notes
    var approvalWindow: NSWindow?
    var approvalWebView: WKWebView?
    var approvalRequestId: String?

    func applicationDidFinishLaunching(_ notification: Notification) {
        tlog("App launched")
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
        tlog("statusItem created, button=\(statusItem.button != nil)")
        let center = UNUserNotificationCenter.current()
        center.delegate = self
        center.requestAuthorization(options: [.alert, .sound]) { granted, _ in
            if granted {
                // Category 1: existing credential — just approve/deny
                let approve = UNNotificationAction(identifier: "APPROVE", title: "Allow", options: [])
                let deny = UNNotificationAction(identifier: "DENY", title: "Deny", options: [.destructive])
                let approveCategory = UNNotificationCategory(
                    identifier: "SECRET_APPROVE",
                    actions: [approve, deny],
                    intentIdentifiers: [],
                    options: []
                )

                // Category 2: new credential — text input for secret value
                let approveWithValue = UNTextInputNotificationAction(
                    identifier: "APPROVE_WITH_VALUE",
                    title: "Allow",
                    options: [],
                    textInputButtonTitle: "Send",
                    textInputPlaceholder: "Enter secret value"
                )
                let denyInput = UNNotificationAction(identifier: "DENY", title: "Deny", options: [.destructive])
                let inputCategory = UNNotificationCategory(
                    identifier: "SECRET_INPUT",
                    actions: [approveWithValue, denyInput],
                    intentIdentifiers: [],
                    options: []
                )

                center.setNotificationCategories([approveCategory, inputCategory])
            }
        }

        // Auto-register as login item (idempotent, no prompt)
        if #available(macOS 13.0, *) {
            let service = SMAppService.mainApp
            if service.status != .enabled {
                try? service.register()
                tlog("Registered as login item")
            }
        }

        updateMenu()
        updateIcon()
        checkHealth()
        healthTimer = Timer.scheduledTimer(timeInterval: 5.0, target: self,
                                           selector: #selector(checkHealth),
                                           userInfo: nil, repeats: true)
    }

    @objc func checkHealth() {
        guard let info = discoverDaemon() else {
            setDisconnected()
            return
        }
        daemon = info

        guard let url = URL(string: "\(info.addr)/api/health") else {
            setDisconnected()
            return
        }

        let task = URLSession.shared.dataTask(with: url) { [weak self] _, response, error in
            DispatchQueue.main.async {
                if let http = response as? HTTPURLResponse, http.statusCode == 200, error == nil {
                    self?.setConnected()
                } else {
                    self?.setDisconnected()
                }
            }
        }
        task.resume()
    }

    func setConnected() {
        let wasConnected = connected
        connected = true
        if !wasConnected {
            tlog("Daemon connected")
            updateMenu()
        }
        if wsTask == nil {
            connectWebSocket()
        }
    }

    func setDisconnected() {
        guard connected else { return }
        tlog("Daemon disconnected")
        connected = false
        daemon = nil
        disconnectWebSocket()
        pendingRequests.removeAll()
        updateIcon()
        updateMenu()
    }

    func connectWebSocket() {
        guard let info = daemon else { return }
        let wsURL = URL(string: "ws://127.0.0.1:\(info.port)/api/ws")!
        wsTask = URLSession.shared.webSocketTask(with: wsURL)
        wsTask?.resume()
        receiveMessage()
        fetchPendingRequests()
        refreshCredentials()
    }

    // refreshCredentials loads the projects to show and their credential
    // aliases for the click-to-copy menu. Values are never fetched — only
    // aliases. Active agent sessions are preferred; if none are running it
    // falls back to all known projects so you can always browse and copy.
    func refreshCredentials() {
        guard let info = daemon else { return }
        resolveProjects { [weak self] projects in
            guard let self = self else { return }
            DispatchQueue.main.async {
                self.orderedProjects = projects
            }
            let group = DispatchGroup()
            var creds: [String: [String]] = [:]
            var notes: [String: [(alias: String, body: String)]] = [:]
            let lock = NSLock()
            for p in projects {
                guard let enc = p.slug.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) else { continue }
                if let cu = URL(string: "\(info.addr)/api/projects/\(enc)/credentials") {
                    group.enter()
                    URLSession.shared.dataTask(with: cu) { d, _, _ in
                        defer { group.leave() }
                        guard let d = d,
                              let obj = try? JSONSerialization.jsonObject(with: d) as? [String: Any] else { return }
                        lock.lock(); creds[p.slug] = obj.keys.sorted(); lock.unlock()
                    }.resume()
                }
                if let nu = URL(string: "\(info.addr)/api/projects/\(enc)/notes") {
                    group.enter()
                    URLSession.shared.dataTask(with: nu) { d, _, _ in
                        defer { group.leave() }
                        guard let d = d,
                              let arr = try? JSONSerialization.jsonObject(with: d) as? [[String: Any]] else { return }
                        let parsed = arr.compactMap { n -> (alias: String, body: String)? in
                            guard let a = n["alias"] as? String else { return nil }
                            return (a, n["body"] as? String ?? "")
                        }
                        lock.lock(); notes[p.slug] = parsed; lock.unlock()
                    }.resume()
                }
            }
            group.notify(queue: .main) {
                self.projectCredentials = creds
                self.projectNotes = notes
                self.updateMenu()
            }
        }
    }

    // resolveProjects returns all known projects, with active agent-session
    // projects first, then the rest — each sorted by name within its group.
    func resolveProjects(_ completion: @escaping ([(slug: String, name: String, active: Bool)]) -> Void) {
        guard let info = daemon else { completion([]); return }
        URLSession.shared.dataTask(with: URL(string: "\(info.addr)/api/sessions")!) { [weak self] sdata, _, _ in
            var activeSet = Set<String>()
            var sessionNames: [String: String] = [:]
            if let sdata = sdata,
               let sessions = try? JSONSerialization.jsonObject(with: sdata) as? [[String: Any]] {
                for s in sessions {
                    if let slug = s["project_slug"] as? String {
                        activeSet.insert(slug)
                        if let n = s["project_name"] as? String { sessionNames[slug] = n }
                    }
                }
            }
            guard let info = self?.daemon else { completion([]); return }
            URLSession.shared.dataTask(with: URL(string: "\(info.addr)/api/projects")!) { pdata, _, _ in
                var all: [(slug: String, name: String, active: Bool)] = []
                if let pdata = pdata,
                   let projects = try? JSONSerialization.jsonObject(with: pdata) as? [[String: Any]] {
                    for p in projects {
                        guard let slug = p["slug"] as? String else { continue }
                        let name = (p["name"] as? String) ?? sessionNames[slug] ?? slug
                        all.append((slug, name, activeSet.contains(slug)))
                    }
                }
                all.sort { a, b in
                    if a.active != b.active { return a.active }
                    return a.name.localizedCaseInsensitiveCompare(b.name) == .orderedAscending
                }
                completion(all)
            }.resume()
        }.resume()
    }

    func fetchPendingRequests() {
        guard let info = daemon else { return }
        let url = URL(string: "\(info.addr)/api/requests")!
        URLSession.shared.dataTask(with: url) { [weak self] data, _, _ in
            guard let data = data,
                  let requests = try? JSONSerialization.jsonObject(with: data) as? [[String: Any]] else { return }
            DispatchQueue.main.async {
                self?.pendingRequests.removeAll()
                for req in requests {
                    guard let id = req["id"] as? String else { continue }
                    self?.pendingRequests[id] = RequestData(
                        id: id,
                        kind: req["kind"] as? String ?? "secret_request",
                        alias: req["alias"] as? String ?? "",
                        aliases: req["aliases"] as? [String] ?? [],
                        reason: req["reason"] as? String ?? "",
                        projectPath: req["project_path"] as? String ?? "",
                        projectName: req["project_name"] as? String ?? "",
                        projectSlug: req["project_slug"] as? String ?? "",
                        status: "pending",
                        createdAt: req["created_at"] as? String ?? ""
                    )
                }
                self?.updateIcon()
            }
        }.resume()
    }

    func disconnectWebSocket() {
        wsTask?.cancel(with: .goingAway, reason: nil)
        wsTask = nil
    }

    func receiveMessage() {
        wsTask?.receive { [weak self] result in
            switch result {
            case .success(let message):
                if case .string(let text) = message,
                   let data = text.data(using: .utf8),
                   let msg = WSMessage(from: data) {
                    DispatchQueue.main.async { self?.handleWSMessage(msg) }
                }
                self?.receiveMessage()
            case .failure(let error):
                tlog("WS receive error: \(error.localizedDescription)")
                DispatchQueue.main.async {
                    self?.wsTask = nil
                }
            }
        }
    }

    func handleWSMessage(_ msg: WSMessage) {
        tlog("WS event: \(msg.type)")
        switch msg.type {
        case "request_created":
            guard let data = msg.data,
                  let id = data["id"] as? String,
                  let kind = data["kind"] as? String else { return }
            tlog("Request created: \(id) kind=\(kind)")
            let alias = data["alias"] as? String ?? ""
            let aliases = data["aliases"] as? [String] ?? []
            let projectName = data["project_name"] as? String ?? msg.project ?? ""
            let projectSlug = data["project_slug"] as? String ?? msg.project ?? ""
            let req = RequestData(id: id, kind: kind, alias: alias, aliases: aliases,
                                  reason: data["reason"] as? String ?? "",
                                  projectPath: data["project_path"] as? String ?? "",
                                  projectName: projectName,
                                  projectSlug: projectSlug,
                                  status: "pending",
                                  createdAt: data["created_at"] as? String ?? "")
            pendingRequests[id] = req
            updateIcon()
            updateMenu()
            if kind == "execute_accept" || kind == "clipboard_copy" {
                sendNotification(for: req, project: projectName, hasValue: true)
            } else {
                checkCredentialExists(project: projectSlug, alias: alias) { [weak self] exists in
                    DispatchQueue.main.async {
                        self?.sendNotification(for: req, project: projectName, hasValue: exists)
                    }
                }
            }
        case "request_resolved":
            guard let data = msg.data, let id = data["id"] as? String else { return }
            tlog("Request resolved: \(id) pending=\(pendingRequests.count - 1)")
            pendingRequests.removeValue(forKey: id)
            UNUserNotificationCenter.current().removeDeliveredNotifications(withIdentifiers: [id])
            if id == approvalRequestId {
                // Navigate to next pending request, or close if none left
                if let next = pendingRequests.values.first {
                    navigateApprovalWindow(to: next.id)
                } else {
                    closeApprovalWindow()
                }
            }
            updateIcon()
            updateMenu()
        case "sessions_changed":
            refreshCredentials()
        case "credentials_changed":
            refreshCredentials()
        case "notes_changed":
            refreshCredentials()
        default: break
        }
    }

    func checkCredentialExists(project: String, alias: String, completion: @escaping (Bool) -> Void) {
        guard let info = daemon, !project.isEmpty,
              let encoded = project.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed),
              let url = URL(string: "\(info.addr)/api/projects/\(encoded)/credentials") else {
            completion(false)
            return
        }
        URLSession.shared.dataTask(with: url) { data, _, _ in
            guard let data = data,
                  let creds = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else {
                completion(false)
                return
            }
            completion(creds[alias] != nil)
        }.resume()
    }

    func sendNotification(for req: RequestData, project: String, hasValue: Bool) {
        let content = UNMutableNotificationContent()
        content.sound = .default
        content.userInfo = ["request_id": req.id]

        if req.kind == "clipboard_copy" {
            // Clipboard copy — existing secret, just Allow/Deny
            content.title = "\(project) → clipboard"
            content.body = "Copy \(req.alias)"
            content.categoryIdentifier = "SECRET_APPROVE"
        } else if req.kind == "execute_accept" {
            // Exec request — show command and aliases
            content.title = "\(project) uses secrets"
            content.body = req.aliases.joined(separator: ", ")
            content.categoryIdentifier = "SECRET_APPROVE"
        } else if hasValue {
            // Existing credential — just needs approval
            content.title = "\(project) uses secrets"
            content.body = req.alias
            content.categoryIdentifier = "SECRET_APPROVE"
        } else {
            // New credential — needs value input
            content.title = "\(project) wants secret"
            content.body = req.alias
            if !req.reason.isEmpty {
                content.body += " — \(req.reason)"
            }
            content.categoryIdentifier = "SECRET_INPUT"
        }

        let request = UNNotificationRequest(identifier: req.id, content: content, trigger: nil)
        UNUserNotificationCenter.current().add(request)
    }

    func userNotificationCenter(_ center: UNUserNotificationCenter,
                                didReceive response: UNNotificationResponse,
                                withCompletionHandler completionHandler: @escaping () -> Void) {
        let requestId = response.notification.request.content.userInfo["request_id"] as? String ?? ""
        switch response.actionIdentifier {
        case "APPROVE":
            approveRequest(id: requestId, value: nil, completionHandler: completionHandler)
        case "APPROVE_WITH_VALUE":
            if let textResponse = response as? UNTextInputNotificationResponse {
                let value = textResponse.userText
                if value.isEmpty {
                    openApprovalWindow(requestId: requestId)
                    completionHandler()
                } else {
                    approveRequest(id: requestId, value: value, completionHandler: completionHandler)
                }
            } else {
                openApprovalWindow(requestId: requestId)
                completionHandler()
            }
        case "DENY":
            denyRequest(id: requestId, completionHandler: completionHandler)
        default:
            openApprovalWindow(requestId: requestId)
            completionHandler()
        }
    }

    func userNotificationCenter(_ center: UNUserNotificationCenter,
                                willPresent notification: UNNotification,
                                withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void) {
        completionHandler([.banner, .sound])
    }

    func approveRequest(id: String, value: String?, completionHandler: @escaping () -> Void) {
        guard let info = daemon else { completionHandler(); return }
        let url = URL(string: "\(info.addr)/api/requests/\(id)/approve")!
        var req = URLRequest(url: url)
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        var body: [String: String] = ["policy": "ask_session"]
        if let value = value {
            body["value"] = value
        }
        req.httpBody = try? JSONSerialization.data(withJSONObject: body)
        URLSession.shared.dataTask(with: req) { [weak self] _, response, _ in
            DispatchQueue.main.async {
                if (response as? HTTPURLResponse)?.statusCode != 200 {
                    self?.openApprovalWindow(requestId: id)
                }
                completionHandler()
            }
        }.resume()
    }

    func denyRequest(id: String, completionHandler: @escaping () -> Void) {
        guard let info = daemon else { completionHandler(); return }
        let url = URL(string: "\(info.addr)/api/requests/\(id)/deny")!
        var req = URLRequest(url: url)
        req.httpMethod = "POST"
        URLSession.shared.dataTask(with: req) { _, _, _ in
            completionHandler()
        }.resume()
    }

    // Persistent window + webView — created once, never destroyed
    func ensureApprovalWindow() {
        guard approvalWindow == nil else { return }
        let webView = WKWebView(frame: NSRect(x: 0, y: 0, width: 420, height: 520))
        let window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 420, height: 520),
            styleMask: [.titled, .closable],
            backing: .buffered,
            defer: false
        )
        window.title = "Yucca"
        window.contentView = webView
        window.center()
        window.level = .floating
        window.isReleasedWhenClosed = false  // keep alive after close
        self.approvalWindow = window
        self.approvalWebView = webView
    }

    func openApprovalWindow(requestId: String) {
        guard let info = daemon else { return }
        tlog("Opening approval window for \(requestId)")
        ensureApprovalWindow()
        approvalRequestId = requestId
        let url = URL(string: "\(info.addr)/request/\(requestId)")!
        approvalWebView!.load(URLRequest(url: url))
        approvalWindow!.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
    }

    func navigateApprovalWindow(to requestId: String) {
        guard let info = daemon else {
            openApprovalWindow(requestId: requestId)
            return
        }
        approvalRequestId = requestId
        let url = URL(string: "\(info.addr)/request/\(requestId)")!
        approvalWebView?.load(URLRequest(url: url))
    }

    func closeApprovalWindow() {
        tlog("Closing approval window")
        approvalWebView?.loadHTMLString("", baseURL: nil)
        approvalWindow?.orderOut(nil)
        approvalRequestId = nil
    }

    func updateIcon() {
        let image: NSImage
        if !connected {
            image = svgToImage(yuccaOfflineSVG, fallbackSymbol: "circle.slash")
        } else if !pendingRequests.isEmpty {
            image = svgToImage(yuccaActionSVG, fallbackSymbol: "smallcircle.filled.circle")
        } else {
            image = svgToImage(yuccaIdleSVG, fallbackSymbol: "circle")
        }
        if statusItem.button == nil {
            tlog("WARNING: statusItem.button is nil!")
        }
        statusItem.button?.image = image
        statusItem.isVisible = true
    }

    // sectionHeader builds a small, gray section label. Uses the native menu
    // section-header style on macOS 14+, with a styled fallback below that.
    func sectionHeader(_ title: String) -> NSMenuItem {
        if #available(macOS 14.0, *) {
            return NSMenuItem.sectionHeader(title: title)
        }
        let item = NSMenuItem(title: title, action: nil, keyEquivalent: "")
        item.isEnabled = false
        item.attributedTitle = NSAttributedString(string: title, attributes: [
            .font: NSFont.systemFont(ofSize: 10, weight: .semibold),
            .foregroundColor: NSColor.secondaryLabelColor,
        ])
        return item
    }

    // projectMenuItem builds a project row (icon + credentials submenu).
    func projectMenuItem(_ p: (slug: String, name: String, active: Bool)) -> NSMenuItem {
        let projectItem = NSMenuItem(title: p.name, action: nil, keyEquivalent: "")
        projectItem.image = p.active
            ? menuIcon(folderActiveSVG, fallback: "folder.fill", template: false)
            : menuIcon(folderSVG, fallback: "folder")
        let submenu = NSMenu()
        let aliases = projectCredentials[p.slug] ?? []
        if aliases.isEmpty {
            let none = NSMenuItem(title: "(no credentials)", action: nil, keyEquivalent: "")
            none.isEnabled = false
            submenu.addItem(none)
        } else {
            for alias in aliases {
                let item = NSMenuItem(title: alias, action: #selector(copyCredential(_:)), keyEquivalent: "")
                item.representedObject = "\(p.slug)|\(alias)"
                item.image = menuIcon(keySVG, fallback: "key")
                item.toolTip = "Copy to clipboard (auto-clears, only if unchanged)"
                submenu.addItem(item)
            }
        }
        let notes = projectNotes[p.slug] ?? []
        if !notes.isEmpty {
            submenu.addItem(NSMenuItem.separator())
            let header = NSMenuItem(title: "NOTES", action: nil, keyEquivalent: "")
            header.isEnabled = false
            submenu.addItem(header)
            for note in notes {
                let item = NSMenuItem(title: note.alias, action: #selector(copyNote(_:)), keyEquivalent: "")
                item.representedObject = note.body
                item.image = NSImage(systemSymbolName: "note.text", accessibilityDescription: "note")
                item.toolTip = note.body
                submenu.addItem(item)
            }
        }
        projectItem.submenu = submenu
        return projectItem
    }

    func updateMenu() {
        let menu = NSMenu()
        if connected {
            // Pending model-initiated approvals first.
            var seenProjects = Set<String>()
            for req in pendingRequests.values {
                let name = req.projectName.isEmpty ? req.projectSlug : req.projectName
                if seenProjects.contains(name) { continue }
                seenProjects.insert(name)
                let item = NSMenuItem(title: "⏳ \(name)", action: #selector(openPendingRequest(_:)), keyEquivalent: "")
                item.representedObject = req.id
                menu.addItem(item)
            }
            if !seenProjects.isEmpty {
                menu.addItem(NSMenuItem.separator())
            }

            // Credential browser: ACTIVE SESSIONS, then OTHER PROJECTS.
            let activeProjects = orderedProjects.filter { $0.active }
            let otherProjects = orderedProjects.filter { !$0.active }
            if orderedProjects.isEmpty {
                let item = NSMenuItem(title: "No projects", action: nil, keyEquivalent: "")
                item.isEnabled = false
                menu.addItem(item)
            } else {
                if !activeProjects.isEmpty {
                    menu.addItem(sectionHeader("ACTIVE SESSIONS"))
                    for p in activeProjects { menu.addItem(projectMenuItem(p)) }
                }
                if !otherProjects.isEmpty {
                    if !activeProjects.isEmpty { menu.addItem(NSMenuItem.separator()) }
                    menu.addItem(sectionHeader("OTHER PROJECTS"))
                    for p in otherProjects { menu.addItem(projectMenuItem(p)) }
                }
            }
            menu.addItem(NSMenuItem.separator())
            menu.addItem(NSMenuItem(title: "Open Yucca", action: #selector(openYucca), keyEquivalent: ""))
        } else {
            let item = NSMenuItem(title: "Daemon offline…", action: nil, keyEquivalent: "")
            item.isEnabled = false
            menu.addItem(item)
        }
        menu.addItem(NSMenuItem.separator())
        menu.addItem(NSMenuItem(title: "Quit", action: #selector(quit), keyEquivalent: "q"))
        statusItem.menu = menu
    }

    @objc func openPendingRequest(_ sender: NSMenuItem) {
        guard let requestId = sender.representedObject as? String else { return }
        openApprovalWindow(requestId: requestId)
    }

    @objc func openYucca() {
        guard let info = daemon else { return }
        NSWorkspace.shared.open(URL(string: info.addr)!)
    }

    @objc func copyCredential(_ sender: NSMenuItem) {
        guard let key = sender.representedObject as? String else { return }
        let parts = key.split(separator: "|", maxSplits: 1).map(String.init)
        guard parts.count == 2 else { return }
        copyToClipboard(slug: parts[0], alias: parts[1])
    }

    // copyNote copies a non-secret note body straight to the clipboard. Notes are
    // not secrets, so this skips the daemon clipboard flow (no approval, no auto-clear).
    @objc func copyNote(_ sender: NSMenuItem) {
        guard let body = sender.representedObject as? String else { return }
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(body, forType: .string)
        let content = UNMutableNotificationContent()
        content.title = "Note copied"
        content.body = sender.title
        UNUserNotificationCenter.current().add(
            UNNotificationRequest(identifier: "note-\(sender.title)", content: content, trigger: nil))
    }

    // copyToClipboard asks the daemon to copy the value to the system clipboard.
    // The value goes RAM → pbcopy in the daemon and never reaches this app.
    // "ui": true tells the daemon this is a trusted user-initiated copy, so the
    // model-approval flow is skipped (the click is the authorization).
    func copyToClipboard(slug: String, alias: String) {
        guard let info = daemon,
              let enc = slug.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed),
              let url = URL(string: "\(info.addr)/api/projects/\(enc)/clipboard") else { return }
        var req = URLRequest(url: url)
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try? JSONSerialization.data(withJSONObject: ["alias": alias, "ui": true])
        URLSession.shared.dataTask(with: req) { [weak self] data, response, _ in
            var clearSecs = 30
            if let data = data,
               let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
               let ms = obj["clear_after_ms"] as? Int { clearSecs = ms / 1000 }
            let ok = (response as? HTTPURLResponse)?.statusCode == 200
            DispatchQueue.main.async {
                self?.notifyCopied(alias: alias, ok: ok, clearSecs: clearSecs)
            }
        }.resume()
    }

    func notifyCopied(alias: String, ok: Bool, clearSecs: Int) {
        let content = UNMutableNotificationContent()
        if ok {
            content.title = "Copied to clipboard"
            content.body = "\(alias) — clears in \(clearSecs)s"
        } else {
            content.title = "Copy failed"
            content.body = alias
        }
        let request = UNNotificationRequest(identifier: "copy-\(alias)", content: content, trigger: nil)
        UNUserNotificationCenter.current().add(request)
    }

    @objc func quit() {
        healthTimer?.invalidate()
        disconnectWebSocket()
        NSApp.terminate(nil)
    }
}

let app = NSApplication.shared
app.setActivationPolicy(.accessory)
let appDelegate = AppDelegate()
app.delegate = appDelegate
withExtendedLifetime(appDelegate) {
    app.run()
}
