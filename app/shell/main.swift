// MyPhotoBook_ROAD — Dock 에 보이는 맥 앱 껍데기
// made by. Nevertheless_D
//
// 왜 이 껍데기가 있나요?
//   macOS 27 의 '백그라운드 앱 활동' 규칙: 앱을 종료한 뒤에도 창·메뉴 막대·Dock 없이 남아 도는 프로그램은
//   사용자에게 보여 주고, 허용되지 않으면 시스템이 끕니다. 예외는 '그 프로그램을 켠 앱이 Dock 에 있을 때'예요.
//   그래서 MyPhotoBook_ROAD 는 Dock 에 보이는 정식 앱이고, 엔진(과 iPhone 도우미)은 이 앱의 자식으로 '앱이 켜져 있는 동안만' 돌아요.
//   규칙을 피해 가는 장치(종료 신호 무시, 몰래 도는 도우미 앱)는 쓰지 않아요.
//
// 하는 일
//   1) 앱 안의 엔진 · iPhone 도우미를 ~/Library/Application Support/MyPhotoBook_ROAD/bin 으로 복사 (업데이트로 앱 묶음이 바뀌어도 안전하게)
//   2) 엔진(지킴이 모드)을 자식으로 켜고, 127.0.0.1:47931 + 토큰 주소를 앱 창(WKWebView)에 띄움
//   3) Dock 아이콘을 누르면 창을 다시 보여 주고, 앱을 종료하면 엔진도 깔끔하게 끔
//   4) 엔진이 끝나면 앱도 끝남 (화면의 '종료' 버튼) · 업데이트 후에는 새 버전을 다시 엶
//   5) 가져오는 동안에는 맥이 잠자기에 들어가지 않게 해요 (화면은 꺼져도 돼요)
//   6) 모든 단계를 launcher.log 에 기록

import AppKit
import Darwin
import WebKit

let appName = "MyPhotoBook_ROAD"

// MARK: - 경로 · 기록

enum Paths {
    static let support: URL = {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
        return base.appendingPathComponent(appName, isDirectory: true)
    }()
    static let bin = support.appendingPathComponent("bin", isDirectory: true)
    static let engine = bin.appendingPathComponent("mpbroad-engine")
    static let device = bin.appendingPathComponent("mpbroad-device")
    static func file(_ name: String) -> URL { support.appendingPathComponent(name) }
}

enum Log {
    static func write(_ message: String) {
        let fm = FileManager.default
        try? fm.createDirectory(at: Paths.support, withIntermediateDirectories: true)
        let url = Paths.file("launcher.log")
        if let attrs = try? fm.attributesOfItem(atPath: url.path), let size = attrs[.size] as? NSNumber, size.intValue > 262_144 {
            let prev = Paths.file("launcher.prev.log")
            try? fm.removeItem(at: prev)
            try? fm.moveItem(at: url, to: prev)
        }
        let df = DateFormatter()
        df.locale = Locale(identifier: "en_US_POSIX")
        df.dateFormat = "MM-dd HH:mm:ss"
        let line = "\(df.string(from: Date())) [앱 \(getpid())] \(message)\n"
        guard let data = line.data(using: .utf8) else { return }
        if let h = try? FileHandle(forWritingTo: url) {
            h.seekToEndOfFile()
            h.write(data)
            h.closeFile()
        } else {
            try? data.write(to: url)
        }
    }
}

// MARK: - 엔진

final class Engine {
    var process: Process?
    var port: Int = 0
    var token: String = ""
    let build: String
    var onExit: ((Int32, Bool) -> Void)?

    init() {
        let v = Bundle.main.url(forResource: "version", withExtension: "txt")
            .flatMap { try? String(contentsOf: $0, encoding: .utf8) } ?? "0.0.0"
        build = v.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private func readText(_ name: String) -> String? {
        guard let s = try? String(contentsOf: Paths.file(name), encoding: .utf8) else { return nil }
        let t = s.trimmingCharacters(in: .whitespacesAndNewlines)
        return t.isEmpty ? nil : t
    }

    private func install(_ resource: String, to dest: URL) throws {
        let fm = FileManager.default
        guard let src = Bundle.main.url(forResource: resource, withExtension: nil) else {
            throw NSError(domain: appName, code: 1, userInfo: [NSLocalizedDescriptionKey: "앱 안에서 \(resource)를 찾지 못했어요"])
        }
        let tmp = dest.appendingPathExtension("new")
        try? fm.removeItem(at: tmp)
        try fm.copyItem(at: src, to: tmp)
        chmod(tmp.path, 0o755)
        removexattr(tmp.path, "com.apple.quarantine", 0)
        if rename(tmp.path, dest.path) != 0 {
            throw NSError(domain: appName, code: 2, userInfo: [NSLocalizedDescriptionKey: "\(resource)를 준비하지 못했어요"])
        }
    }

    /// 앱 안의 엔진 · iPhone 도우미를 앱 전용 폴더로 복사해요 (버전이 바뀌었을 때만)
    func prepare() throws {
        let fm = FileManager.default
        try fm.createDirectory(at: Paths.bin, withIntermediateDirectories: true)
        let buildFile = Paths.bin.appendingPathComponent("build")
        let installed = (try? String(contentsOf: buildFile, encoding: .utf8))?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let want = build + "-native"
        if installed != want || !fm.isExecutableFile(atPath: Paths.engine.path) || !fm.isExecutableFile(atPath: Paths.device.path) {
            try install("mpbroad-engine", to: Paths.engine)
            try install("mpbroad-device", to: Paths.device)
            try want.write(to: buildFile, atomically: true, encoding: .utf8)
            Log.write("엔진 · iPhone 도우미 준비 (v\(build))")
        }
        removexattr(Paths.engine.path, "com.apple.quarantine", 0)
        removexattr(Paths.device.path, "com.apple.quarantine", 0)
    }

    /// 127.0.0.1 엔진에 짧게 요청해요 (응답이 200 이면 내용을 돌려줘요)
    func request(_ method: String, port: Int, path: String, token: String, timeout: TimeInterval) -> Data? {
        guard let url = URL(string: "http://127.0.0.1:\(port)\(path)") else { return nil }
        var req = URLRequest(url: url, cachePolicy: .reloadIgnoringLocalCacheData, timeoutInterval: timeout)
        req.httpMethod = method
        req.setValue(token, forHTTPHeaderField: "x-token")
        if method == "POST" {
            req.httpBody = "{}".data(using: .utf8)
            req.setValue("application/json", forHTTPHeaderField: "content-type")
        }
        let sem = DispatchSemaphore(value: 0)
        var out: Data?
        let task = URLSession.shared.dataTask(with: req) { data, resp, _ in
            if let h = resp as? HTTPURLResponse, h.statusCode == 200 { out = data ?? Data() }
            sem.signal()
        }
        task.resume()
        if sem.wait(timeout: .now() + timeout + 0.5) == .timedOut { task.cancel() }
        return out
    }

    /// 이미 켜져 있는 엔진이 있으면 끄고 새로 켜요
    func stopExisting() {
        guard let p = readText("port").flatMap({ Int($0) }), let t = readText("token") else { return }
        guard request("GET", port: p, path: "/api/ping", token: t, timeout: 1) != nil else { return }
        Log.write("이미 켜져 있던 엔진(포트 \(p))을 끄고 새로 켜요")
        _ = request("POST", port: p, path: "/api/quit", token: t, timeout: 2)
        for _ in 0..<40 {
            if request("GET", port: p, path: "/api/ping", token: t, timeout: 0.5) == nil { break }
            usleep(150_000)
        }
    }

    func start() throws {
        let fm = FileManager.default
        token = readText("token") ?? Engine.randomToken()
        try? fm.removeItem(at: Paths.file("port"))
        try? fm.removeItem(at: Paths.file("restart-requested"))

        let p = Process()
        p.executableURL = Paths.engine
        var env = ProcessInfo.processInfo.environment
        for k in ["MPB_RESTART", "MPB_PORT", "MPB_CHILD"] { env.removeValue(forKey: k) }
        env["MPB_SUPERVISOR"] = "1"
        env["MPB_SHELL"] = "native"
        env["MPB_VIA"] = "native"
        env["MPB_TOKEN"] = token
        env["MPB_SUPPORT"] = Paths.support.path
        env["MPB_APP"] = Bundle.main.bundlePath
        env["MPB_PARENT_PID"] = String(getpid())
        env["MPB_PORT_WAIT"] = "1"
        p.environment = env
        p.standardInput = FileHandle.nullDevice
        p.standardOutput = FileHandle.nullDevice
        p.standardError = FileHandle.nullDevice
        p.terminationHandler = { [weak self] proc in
            let code = proc.terminationStatus
            DispatchQueue.main.async { self?.exited(code) }
        }
        try p.run()
        process = p
        Log.write("엔진 켬 (지킴이 pid \(p.processIdentifier))")
    }

    func waitForPort() -> Int? {
        for _ in 0..<100 {
            if let v = readText("port").flatMap({ Int($0) }) {
                port = v
                return v
            }
            usleep(150_000)
        }
        return nil
    }

    private func exited(_ code: Int32) {
        let restart = FileManager.default.fileExists(atPath: Paths.file("restart-requested").path)
        process = nil
        onExit?(code, restart)
    }

    /// 앱을 종료할 때: 엔진에게 정중히 끄라고 하고, 안 꺼지면 종료 신호를 보내요
    func stop(completion: @escaping () -> Void) {
        guard let p = process, p.isRunning else { completion(); return }
        let port = self.port, token = self.token
        DispatchQueue.global(qos: .userInitiated).async {
            _ = self.request("POST", port: port, path: "/api/quit", token: token, timeout: 2)
            for _ in 0..<40 where p.isRunning { usleep(100_000) }
            if p.isRunning {
                p.terminate()
                for _ in 0..<20 where p.isRunning { usleep(100_000) }
            }
            if p.isRunning { kill(p.processIdentifier, SIGKILL) }
            DispatchQueue.main.async { completion() }
        }
    }

    /// 지금 가져오는 중인지 (잠자기 막기 · 종료 확인에 써요)
    func jobStage() -> String {
        guard port > 0, let d = request("GET", port: port, path: "/api/poll", token: token, timeout: 1.5),
              let obj = try? JSONSerialization.jsonObject(with: d) as? [String: Any],
              let job = obj["job"] as? [String: Any] else { return "" }
        return (job["stage"] as? String) ?? ""
    }

    static func randomToken() -> String {
        let chars = Array("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")
        return String((0..<32).map { _ in chars[Int(arc4random_uniform(UInt32(chars.count)))] })
    }
}

// MARK: - 앱

final class AppDelegate: NSObject, NSApplicationDelegate, WKNavigationDelegate, WKUIDelegate, NSWindowDelegate {
    var window: NSWindow!
    var web: WKWebView!
    let engine = Engine()
    var quitting = false
    var pageURL: URL?
    var awake: NSObjectProtocol?
    var watchTimer: Timer?

    func applicationDidFinishLaunching(_ notification: Notification) {
        Log.write("시작 v\(engine.build) · macOS \(ProcessInfo.processInfo.operatingSystemVersionString) · \(Bundle.main.bundlePath)")
        buildMenu()
        makeWindow()
        showLoading()
        engine.onExit = { [weak self] code, restart in self?.engineExited(code: code, restart: restart) }
        boot()
    }

    func boot() {
        DispatchQueue.global(qos: .userInitiated).async {
            do {
                try self.engine.prepare()
                self.engine.stopExisting()
                try self.engine.start()
                guard let port = self.engine.waitForPort() else {
                    throw NSError(domain: appName, code: 3, userInfo: [NSLocalizedDescriptionKey: "엔진이 준비되지 않았어요"])
                }
                Log.write("엔진 준비됨 (포트 \(port))")
                DispatchQueue.main.async {
                    self.loadApp(port: port)
                    self.startWatching()
                }
            } catch {
                Log.write("⚠️ 시작 실패: \(error.localizedDescription)")
                DispatchQueue.main.async { self.startFailed(error) }
            }
        }
    }

    func loadApp(port: Int) {
        guard let url = URL(string: "http://127.0.0.1:\(port)/?t=\(engine.token)") else { return }
        pageURL = url
        web.load(URLRequest(url: url))
    }

    func startFailed(_ error: Error) {
        let a = NSAlert()
        a.alertStyle = .warning
        a.messageText = "\(appName)를 시작하지 못했어요"
        a.informativeText = "\(error.localizedDescription)\n앱을 다시 실행해 보세요. 계속 안 되면 맥을 재시동한 뒤 시도해 주세요."
        a.addButton(withTitle: "다시 시도")
        a.addButton(withTitle: "종료")
        if a.runModal() == .alertFirstButtonReturn {
            boot()
        } else {
            quitting = true
            NSApp.terminate(nil)
        }
    }

    func engineExited(code: Int32, restart: Bool) {
        if quitting { return }
        if restart {
            relaunch()
            return
        }
        Log.write("엔진이 끝나서 앱도 닫아요 (코드 \(code))")
        quitting = true
        NSApp.terminate(nil)
    }

    /// 업데이트된 새 버전을 macOS 정식 방법(LaunchServices)으로 다시 열고, 지금 앱은 끝내요
    func relaunch() {
        Log.write("새 버전으로 다시 열어요")
        quitting = true
        let appURL = URL(fileURLWithPath: Bundle.main.bundlePath)
        let cfg = NSWorkspace.OpenConfiguration()
        cfg.createsNewApplicationInstance = true
        cfg.activates = true
        var env = ProcessInfo.processInfo.environment
        env.removeValue(forKey: "MPB_RESTART")
        env["MPB_RELAUNCHED"] = "1"
        cfg.environment = env
        NSWorkspace.shared.openApplication(at: appURL, configuration: cfg) { _, error in
            if let error = error { Log.write("⚠️ 다시 열지 못했어요: \(error.localizedDescription)") }
            DispatchQueue.main.async { NSApp.terminate(nil) }
        }
    }

    // 가져오는 동안 맥이 잠들지 않게 해요 (5초마다 확인)
    func startWatching() {
        watchTimer?.invalidate()
        watchTimer = Timer.scheduledTimer(withTimeInterval: 5, repeats: true) { [weak self] _ in
            guard let self = self, !self.quitting else { return }
            DispatchQueue.global(qos: .utility).async {
                let st = self.engine.jobStage()
                let busy = ["running", "locked", "waiting", "deleting", "problem"].contains(st)
                DispatchQueue.main.async { self.keepAwake(busy) }
            }
        }
    }

    func keepAwake(_ on: Bool) {
        if on && awake == nil {
            awake = ProcessInfo.processInfo.beginActivity(options: [.idleSystemSleepDisabled, .userInitiated],
                                                          reason: "iPhone 사진을 맥으로 옮기는 중")
            Log.write("가져오는 중 → 맥 잠자기 막음")
        } else if !on, let a = awake {
            ProcessInfo.processInfo.endActivity(a)
            awake = nil
            Log.write("가져오기 끝 → 잠자기 막기 풀기")
        }
    }

    // Dock 아이콘을 누르면 창을 다시 보여 줘요
    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        if !flag { showMain() }
        return true
    }

    // 창을 닫아도 앱은 Dock 에 남아요 (Dock 아이콘을 누르면 다시 열림)
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { false }

    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        guard engine.process != nil else { return .terminateNow }
        if !quitting && awake != nil && ProcessInfo.processInfo.environment["MPB_NO_QUIT_ASK"] != "1" {
            let a = NSAlert()
            a.messageText = "사진을 옮기는 중이에요"
            a.informativeText = "지금 끄면 여기까지 옮긴 사진은 맥에 그대로 있고, 다음에 앱을 열면 멈춘 곳부터 이어서 해요."
            a.addButton(withTitle: "끄기")
            a.addButton(withTitle: "계속 옮기기")
            if a.runModal() != .alertFirstButtonReturn { return .terminateCancel }
        }
        quitting = true
        keepAwake(false)
        Log.write("종료: 엔진을 끄는 중")
        engine.stop {
            Log.write("종료 완료")
            NSApp.reply(toApplicationShouldTerminate: true)
        }
        return .terminateLater
    }

    // MARK: 창

    func makeWindow() {
        let cfg = WKWebViewConfiguration()
        cfg.applicationNameForUserAgent = "MPBROADShell/\(engine.build)"
        web = WKWebView(frame: NSRect(x: 0, y: 0, width: 1280, height: 860), configuration: cfg)
        web.navigationDelegate = self
        web.uiDelegate = self
        web.allowsMagnification = false
        if #available(macOS 13.3, *) { web.isInspectable = true }

        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 1280, height: 860),
                          styleMask: [.titled, .closable, .miniaturizable, .resizable],
                          backing: .buffered, defer: false)
        window.title = appName
        window.minSize = NSSize(width: 820, height: 600)
        window.isReleasedWhenClosed = false
        window.tabbingMode = .disallowed
        window.collectionBehavior.insert(.fullScreenPrimary)
        window.delegate = self
        window.contentView = web
        if !window.setFrameUsingName("MPBROADMain") { window.center() }
        window.setFrameAutosaveName("MPBROADMain")
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
    }

    func showLoading() {
        let html = """
        <!doctype html><meta charset="utf-8"><style>
        :root{color-scheme:light dark}
        body{margin:0;height:100vh;display:grid;place-items:center;font:600 14px -apple-system,"Apple SD Gothic Neo",sans-serif;
             background:#EEF1F5;color:#556170}
        @media (prefers-color-scheme: dark){body{background:#0C0F14;color:#9AA6B5}}
        .w{display:flex;flex-direction:column;align-items:center;gap:14px}
        .s{width:22px;height:22px;border-radius:50%;border:2.5px solid rgba(128,140,160,.3);border-top-color:#2D5BFF;animation:r .9s linear infinite}
        @keyframes r{to{transform:rotate(360deg)}}
        </style><div class="w"><div class="s"></div>MyPhotoBook_ROAD 준비 중…</div>
        """
        web.loadHTMLString(html, baseURL: nil)
    }

    @objc func showMain() {
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
    }

    @objc func openInBrowser() {
        if let u = pageURL { NSWorkspace.shared.open(u) }
    }

    @objc func openDiag() {
        if engine.port > 0, let u = URL(string: "http://127.0.0.1:\(engine.port)/diag") { NSWorkspace.shared.open(u) }
    }

    @objc func openPhotosFolder() {
        let pics = FileManager.default.urls(for: .picturesDirectory, in: .userDomainMask)[0].appendingPathComponent("MyPhotoBook")
        try? FileManager.default.createDirectory(at: pics, withIntermediateDirectories: true)
        NSWorkspace.shared.open(pics)
    }

    @objc func reloadPage() {
        if let u = pageURL { web.load(URLRequest(url: u)) }
    }

    // MARK: WKWebView

    func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction,
                 decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        guard let u = action.request.url else { decisionHandler(.allow); return }
        let scheme = u.scheme ?? ""
        if u.path == "/diag" && action.navigationType == .linkActivated {
            NSWorkspace.shared.open(u)
            decisionHandler(.cancel)
            return
        }
        if scheme == "about" || scheme == "data" || scheme == "blob" || u.host == "127.0.0.1" || u.host == "localhost" {
            decisionHandler(.allow)
            return
        }
        // 바깥 주소는 기본 브라우저로 열어요
        NSWorkspace.shared.open(u)
        decisionHandler(.cancel)
    }

    func webView(_ webView: WKWebView, createWebViewWith configuration: WKWebViewConfiguration,
                 for action: WKNavigationAction, windowFeatures: WKWindowFeatures) -> WKWebView? {
        if let u = action.request.url {
            // 진단 정보처럼 새 창으로 여는 것은 기본 브라우저로
            NSWorkspace.shared.open(u)
        }
        return nil
    }

    func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) {
        guard pageURL != nil, !quitting else { return }
        Log.write("⚠️ 화면을 불러오지 못함: \(error.localizedDescription) → 다시 시도")
        DispatchQueue.main.asyncAfter(deadline: .now() + 1.0) { [weak self] in self?.reloadPage() }
    }

    func webViewWebContentProcessDidTerminate(_ webView: WKWebView) {
        Log.write("⚠️ 화면 프로세스가 멈춤 → 다시 불러와요")
        reloadPage()
    }

    func webViewDidClose(_ webView: WKWebView) {
        window.orderOut(nil)
    }

    func webView(_ webView: WKWebView, runJavaScriptAlertPanelWithMessage message: String,
                 initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping () -> Void) {
        let a = NSAlert()
        a.messageText = message
        a.addButton(withTitle: "확인")
        a.beginSheetModal(for: window) { _ in completionHandler() }
    }

    func webView(_ webView: WKWebView, runJavaScriptConfirmPanelWithMessage message: String,
                 initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping (Bool) -> Void) {
        let a = NSAlert()
        a.messageText = message
        a.addButton(withTitle: "확인")
        a.addButton(withTitle: "취소")
        a.beginSheetModal(for: window) { r in completionHandler(r == .alertFirstButtonReturn) }
    }

    // MARK: 메뉴

    func buildMenu() {
        let main = NSMenu()

        let appItem = NSMenuItem()
        main.addItem(appItem)
        let appMenu = NSMenu()
        appMenu.addItem(withTitle: "\(appName)에 관하여", action: #selector(NSApplication.orderFrontStandardAboutPanel(_:)), keyEquivalent: "")
        appMenu.addItem(NSMenuItem.separator())
        appMenu.addItem(withTitle: "\(appName) 가리기", action: #selector(NSApplication.hide(_:)), keyEquivalent: "h")
        let others = appMenu.addItem(withTitle: "기타 가리기", action: #selector(NSApplication.hideOtherApplications(_:)), keyEquivalent: "h")
        others.keyEquivalentModifierMask = [.command, .option]
        appMenu.addItem(withTitle: "모두 보기", action: #selector(NSApplication.unhideAllApplications(_:)), keyEquivalent: "")
        appMenu.addItem(NSMenuItem.separator())
        appMenu.addItem(withTitle: "\(appName) 종료", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        appItem.submenu = appMenu

        let editItem = NSMenuItem()
        main.addItem(editItem)
        let edit = NSMenu(title: "편집")
        edit.addItem(withTitle: "실행 취소", action: Selector(("undo:")), keyEquivalent: "z")
        let redo = edit.addItem(withTitle: "실행 복귀", action: Selector(("redo:")), keyEquivalent: "z")
        redo.keyEquivalentModifierMask = [.command, .shift]
        edit.addItem(NSMenuItem.separator())
        edit.addItem(withTitle: "오려두기", action: #selector(NSText.cut(_:)), keyEquivalent: "x")
        edit.addItem(withTitle: "복사하기", action: #selector(NSText.copy(_:)), keyEquivalent: "c")
        edit.addItem(withTitle: "붙여넣기", action: #selector(NSText.paste(_:)), keyEquivalent: "v")
        edit.addItem(withTitle: "모두 선택", action: #selector(NSText.selectAll(_:)), keyEquivalent: "a")
        editItem.submenu = edit

        let viewItem = NSMenuItem()
        main.addItem(viewItem)
        let view = NSMenu(title: "보기")
        let fs = view.addItem(withTitle: "전체 화면", action: #selector(NSWindow.toggleFullScreen(_:)), keyEquivalent: "f")
        fs.keyEquivalentModifierMask = [.command, .control]
        view.addItem(NSMenuItem.separator())
        let pics = view.addItem(withTitle: "사진 › MyPhotoBook 폴더 열기", action: #selector(openPhotosFolder), keyEquivalent: "")
        pics.target = self
        let diag = view.addItem(withTitle: "진단 정보 보기", action: #selector(openDiag), keyEquivalent: "")
        diag.target = self
        viewItem.submenu = view

        let winItem = NSMenuItem()
        main.addItem(winItem)
        let win = NSMenu(title: "윈도우")
        win.addItem(withTitle: "최소화", action: #selector(NSWindow.performMiniaturize(_:)), keyEquivalent: "m")
        win.addItem(withTitle: "확대/축소", action: #selector(NSWindow.performZoom(_:)), keyEquivalent: "")
        win.addItem(withTitle: "닫기", action: #selector(NSWindow.performClose(_:)), keyEquivalent: "w")
        win.addItem(NSMenuItem.separator())
        let show = win.addItem(withTitle: "\(appName) 창 보기", action: #selector(showMain), keyEquivalent: "")
        show.target = self
        winItem.submenu = win

        NSApp.mainMenu = main
        NSApp.windowsMenu = win
    }
}

// MARK: - 시작

let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
app.setActivationPolicy(.regular)
app.run()
