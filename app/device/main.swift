// MyPhotoBook_ROAD — iPhone 연결 도우미 (mpbroad-device)
// made by. Nevertheless_D
//
// 엔진(mpbroad-engine)의 자식으로만 돌아요. 엔진이 끝나면(stdin 이 닫히면) 바로 끝나요.
// Apple 의 ImageCaptureCore 로 케이블에 연결된 iPhone 의 사진 · 동영상을 읽어요.
//
// 엔진 → 도우미 (stdin, 한 줄에 JSON 하나)
//   {"id":1,"op":"thumb","h":"7","path":"/…/x.jpg","max":320}
//   {"id":2,"op":"fetch","h":"7","path":"/…/x.part","off":0,"size":12345}   받은 만큼 남아 있는 파일을 off 부터 이어 받아요
//   {"id":3,"op":"cancel","target":2}
//   {"id":4,"op":"delete","hs":["7","8"]}
// 도우미 → 엔진 (stdout)
//   {"ev":"status","state":"none|connecting|locked|loading|ready","name":"…","catalogPct":0~100,"canDelete":true}
//   {"ev":"add","items":[{"h","name","size","created","uti","group","folder","dur"}]}  {"ev":"remove","hs":[…]}  {"ev":"reset"}
//   {"ev":"progress","for":2,"done":바이트}   {"re":2,"ok":true,"sha":"…"}   {"re":2,"ok":false,"code":"locked|gone|missing|io","err":"…"}
// 기록: stderr (엔진이 Support/device.log 에 붙여 적어요)

import CryptoKit
import Foundation
import ImageCaptureCore
import ImageIO
import UniformTypeIdentifiers

// MARK: - 주고받기

let outQueue = DispatchQueue(label: "out")

func emit(_ obj: [String: Any]) {
    guard let d = try? JSONSerialization.data(withJSONObject: obj, options: []) else { return }
    outQueue.async {
        FileHandle.standardOutput.write(d)
        FileHandle.standardOutput.write(Data([0x0A]))
    }
}

func say(_ s: String) {
    let df = DateFormatter()
    df.locale = Locale(identifier: "en_US_POSIX")
    df.dateFormat = "MM-dd HH:mm:ss"
    let line = "\(df.string(from: Date())) [도우미 \(getpid())] \(s)\n"
    FileHandle.standardError.write(line.data(using: .utf8) ?? Data())
}

func reply(_ id: Int, ok: Bool, code: String = "", err: String = "", extra: [String: Any] = [:]) {
    var m: [String: Any] = ["re": id, "ok": ok]
    if !code.isEmpty { m["code"] = code }
    if !err.isEmpty { m["err"] = err }
    for (k, v) in extra { m[k] = v }
    emit(m)
}

// MARK: - 도우미

final class Helper: NSObject, ICDeviceBrowserDelegate, ICCameraDeviceDelegate {
    let browser = ICDeviceBrowser()
    var camera: ICCameraDevice?
    var files: [String: ICCameraFile] = [:]
    var handles: [ObjectIdentifier: String] = [:]
    var nextHandle = 0
    var catalogDone = false
    var sessionOpen = false
    var lastStatus = ""
    var thumbWaiters: [ObjectIdentifier: [(CGImage?) -> Void]] = [:]
    var cancelled = Set<Int>()
    var deleting: [(id: Int, want: Set<String>, got: [String], last: Date)] = []

    func start() {
        browser.delegate = self
        let raw = ICDeviceTypeMask.camera.rawValue | ICDeviceLocationTypeMask.local.rawValue
        if let mask = ICDeviceTypeMask(rawValue: raw) as ICDeviceTypeMask? {
            browser.browsedDeviceTypeMask = mask
        }
        browser.start()
        say("iPhone 찾기 시작")
        status(force: true)
        Timer.scheduledTimer(withTimeInterval: 2, repeats: true) { [weak self] _ in
            self?.status()
            self?.checkDeletes()
        }
    }

    // MARK: 상태

    var isLocked: Bool {
        guard let c = camera else { return false }
        if #available(macOS 10.15, *) { return c.isAccessRestrictedAppleDevice }
        return false
    }

    func status(force: Bool = false) {
        var state = "none"
        var name = ""
        var pct = 0
        var canDelete = false
        if let c = camera {
            name = c.name ?? "iPhone"
            canDelete = c.capabilities.contains { $0.contains("Delete") }
            if isLocked {
                state = "locked"
            } else if !sessionOpen {
                state = "connecting"
            } else if !catalogDone {
                state = "loading"
                pct = Int(c.contentCatalogPercentCompleted)
            } else {
                state = "ready"
                pct = 100
            }
        }
        let key = "\(state)|\(name)|\(pct)|\(canDelete)"
        if !force && key == lastStatus { return }
        if key.split(separator: "|").first != lastStatus.split(separator: "|").first {
            say("상태: \(state) \(name)")
        }
        lastStatus = key
        emit(["ev": "status", "state": state, "name": name, "catalogPct": pct, "canDelete": canDelete])
    }

    // MARK: 항목

    func handle(for f: ICCameraFile) -> String {
        let oid = ObjectIdentifier(f)
        if let h = handles[oid] { return h }
        nextHandle += 1
        let h = String(nextHandle)
        handles[oid] = h
        files[h] = f
        return h
    }

    func folderPath(_ item: ICCameraItem) -> String {
        var parts: [String] = []
        var f: ICCameraFolder? = item.parentFolder
        while let x = f {
            if let n = x.name, !n.isEmpty { parts.insert(n, at: 0) }
            f = x.parentFolder
        }
        return parts.joined(separator: "/")
    }

    func info(_ f: ICCameraFile) -> [String: Any] {
        var created: Date? = nil
        if #available(macOS 10.15, *) {
            created = f.exifCreationDate
            if created == nil { created = f.fileCreationDate }
        }
        if created == nil { created = f.creationDate }
        var m: [String: Any] = [
            "h": handle(for: f),
            "name": f.name ?? "",
            "size": Int64(f.fileSize),
            "created": (created ?? Date()).timeIntervalSince1970,
            "uti": f.uti ?? "",
            "folder": folderPath(f),
        ]
        if #available(macOS 10.15, *) {
            if let g = f.groupUUID { m["group"] = g }
            if f.duration > 0 { m["dur"] = f.duration }
        }
        return m
    }

    func addItems(_ items: [ICCameraItem]) {
        var out: [[String: Any]] = []
        for it in items {
            guard let f = it as? ICCameraFile else { continue }
            out.append(info(f))
            if out.count >= 500 {
                emit(["ev": "add", "items": out])
                out.removeAll()
            }
        }
        if !out.isEmpty { emit(["ev": "add", "items": out]) }
    }

    func resetItems() {
        files.removeAll()
        handles.removeAll()
        catalogDone = false
        emit(["ev": "reset"])
    }

    // MARK: ICDeviceBrowserDelegate

    func deviceBrowser(_ browser: ICDeviceBrowser, didAdd device: ICDevice, moreComing: Bool) {
        guard let c = device as? ICCameraDevice else { return }
        if camera != nil { return }
        say("기기 연결: \(c.name ?? "?") (\(c.productKind ?? "?"))")
        camera = c
        c.delegate = self
        sessionOpen = false
        resetItems()
        c.requestOpenSession()
        status(force: true)
    }

    func deviceBrowser(_ browser: ICDeviceBrowser, didRemove device: ICDevice, moreGoing: Bool) {
        if device === camera { gone("기기 빠짐") }
    }

    func gone(_ why: String) {
        say(why)
        camera = nil
        sessionOpen = false
        resetItems()
        for (_, ws) in thumbWaiters { ws.forEach { $0(nil) } }
        thumbWaiters.removeAll()
        status(force: true)
    }

    // MARK: ICDeviceDelegate

    func didRemove(_ device: ICDevice) {
        if device === camera { gone("기기 사라짐") }
    }

    func device(_ device: ICDevice, didOpenSessionWithError error: Error?) {
        if let e = error {
            say("세션 열기 실패: \(e.localizedDescription)")
            DispatchQueue.main.asyncAfter(deadline: .now() + 3) { [weak self] in
                if let c = self?.camera, c === device, self?.sessionOpen == false { c.requestOpenSession() }
            }
        } else {
            say("세션 열림")
            sessionOpen = true
        }
        status(force: true)
    }

    func device(_ device: ICDevice, didCloseSessionWithError error: Error?) {
        say("세션 닫힘 \(error?.localizedDescription ?? "")")
        sessionOpen = false
        status(force: true)
    }

    func device(_ device: ICDevice, didEncounterError error: Error?) {
        say("기기 오류: \(error?.localizedDescription ?? "?")")
    }

    // MARK: ICCameraDeviceDelegate

    func cameraDevice(_ camera: ICCameraDevice, didAdd items: [ICCameraItem]) {
        addItems(items)
        status()
    }

    func cameraDevice(_ camera: ICCameraDevice, didRemove items: [ICCameraItem]) {
        var hs: [String] = []
        for it in items {
            let oid = ObjectIdentifier(it)
            if let h = handles[oid] {
                hs.append(h)
                handles.removeValue(forKey: oid)
                files.removeValue(forKey: h)
            }
        }
        if !hs.isEmpty {
            emit(["ev": "remove", "hs": hs])
            for i in deleting.indices {
                for h in hs where deleting[i].want.contains(h) {
                    deleting[i].got.append(h)
                    deleting[i].last = Date()
                }
            }
            checkDeletes()
        }
    }

    func cameraDevice(_ camera: ICCameraDevice, didReceiveThumbnail thumbnail: CGImage?, for item: ICCameraItem, error: Error?) {
        let oid = ObjectIdentifier(item)
        let ws = thumbWaiters.removeValue(forKey: oid) ?? []
        ws.forEach { $0(thumbnail) }
    }

    func cameraDevice(_ camera: ICCameraDevice, didReceiveMetadata metadata: [AnyHashable: Any]?, for item: ICCameraItem, error: Error?) {}

    func cameraDevice(_ camera: ICCameraDevice, didRenameItems items: [ICCameraItem]) {}

    func cameraDeviceDidChangeCapability(_ camera: ICCameraDevice) { status() }

    func cameraDevice(_ camera: ICCameraDevice, didReceivePTPEvent eventData: Data) {}

    func deviceDidBecomeReady(withCompleteContentCatalog device: ICCameraDevice) {
        say("사진 목록 다 읽음: \(files.count)개")
        // 혹시 빠진 항목이 있으면 채워요
        if let all = device.mediaFiles {
            let missing = all.filter { handles[ObjectIdentifier($0)] == nil }
            if !missing.isEmpty { addItems(missing) }
        }
        catalogDone = true
        status(force: true)
    }

    func cameraDeviceDidRemoveAccessRestriction(_ device: ICDevice) {
        say("iPhone 잠금 풀림")
        status(force: true)
    }

    func cameraDeviceDidEnableAccessRestriction(_ device: ICDevice) {
        say("iPhone 잠김")
        status(force: true)
    }

    // MARK: 부탁 처리

    func failCode() -> String {
        if camera == nil { return "gone" }
        if isLocked { return "locked" }
        return "io"
    }

    func run(_ req: [String: Any]) {
        let id = (req["id"] as? Int) ?? Int((req["id"] as? Double) ?? 0)
        let op = (req["op"] as? String) ?? ""
        switch op {
        case "thumb": thumb(id, req)
        case "fetch": fetch(id, req)
        case "cancel":
            let t = (req["target"] as? Int) ?? Int((req["target"] as? Double) ?? 0)
            cancelled.insert(t)
            reply(id, ok: true)
        case "delete": delete(id, req)
        case "ping": reply(id, ok: true)
        default: reply(id, ok: false, code: "io", err: "알 수 없는 부탁")
        }
    }

    func file(_ req: [String: Any]) -> ICCameraFile? {
        guard let h = req["h"] as? String else { return nil }
        return files[h]
    }

    func thumb(_ id: Int, _ req: [String: Any]) {
        guard let f = file(req), let path = req["path"] as? String else {
            reply(id, ok: false, code: camera == nil ? "gone" : "missing", err: "항목 없음")
            return
        }
        if isLocked { reply(id, ok: false, code: "locked"); return }
        let max = (req["max"] as? Int) ?? 320
        let done: (CGImage?) -> Void = { img in
            guard let img = img, writeJPEG(img, to: path, max: max) else {
                reply(id, ok: false, code: "io", err: "미리보기를 받지 못했어요")
                return
            }
            reply(id, ok: true)
        }
        if let t = f.thumbnail {
            done(t)
            return
        }
        let oid = ObjectIdentifier(f)
        let first = thumbWaiters[oid] == nil
        thumbWaiters[oid, default: []].append(done)
        if first { f.requestThumbnail() }
    }

    func fetch(_ id: Int, _ req: [String: Any]) {
        guard let f = file(req), let path = req["path"] as? String else {
            reply(id, ok: false, code: camera == nil ? "gone" : "missing", err: "항목 없음")
            return
        }
        if isLocked { reply(id, ok: false, code: "locked"); return }
        let off = Int64((req["off"] as? Double) ?? Double((req["off"] as? Int) ?? 0))
        let size = Int64((req["size"] as? Double) ?? Double((req["size"] as? Int) ?? Int(f.fileSize)))
        let fm = FileManager.default
        if !fm.fileExists(atPath: path) { fm.createFile(atPath: path, contents: nil) }
        guard let fh = FileHandle(forUpdatingAtPath: path) else {
            reply(id, ok: false, code: "io", err: "임시 파일을 열지 못했어요")
            return
        }
        var hasher = SHA256()
        do {
            try fh.truncate(atOffset: UInt64(off))
            try fh.seek(toOffset: 0)
            var left = off
            while left > 0 {
                let n = Int(min(left, 8 << 20))
                guard let d = try fh.read(upToCount: n), !d.isEmpty else { break }
                hasher.update(data: d)
                left -= Int64(d.count)
            }
            try fh.seekToEnd()
        } catch {
            try? fh.close()
            reply(id, ok: false, code: "io", err: "임시 파일 준비 실패: \(error.localizedDescription)")
            return
        }
        var pos = off
        var lastEmit = Date.distantPast
        let chunk: Int64 = 4 << 20

        func finish(ok: Bool, code: String = "", err: String = "") {
            try? fh.synchronize()
            try? fh.close()
            cancelled.remove(id)
            if ok {
                let sha = hasher.finalize().map { String(format: "%02x", $0) }.joined()
                reply(id, ok: true, extra: ["sha": sha, "size": pos])
            } else {
                reply(id, ok: false, code: code, err: err)
            }
        }

        func step() {
            if cancelled.contains(id) { finish(ok: false, code: "io", err: "취소"); return }
            if pos >= size { finish(ok: true); return }
            guard camera != nil else { finish(ok: false, code: "gone", err: "연결 끊김"); return }
            if isLocked { finish(ok: false, code: "locked", err: "잠김"); return }
            let len = min(chunk, size - pos)
            f.requestReadData(atOffset: off_t(pos), length: off_t(len)) { data, error in
                DispatchQueue.main.async {
                    if let e = error {
                        say("읽기 실패 \(f.name ?? "") @\(pos): \(e.localizedDescription)")
                        finish(ok: false, code: self.failCode(), err: e.localizedDescription)
                        return
                    }
                    guard let d = data, !d.isEmpty else {
                        finish(ok: false, code: self.failCode(), err: "빈 응답")
                        return
                    }
                    do {
                        try fh.write(contentsOf: d)
                    } catch {
                        finish(ok: false, code: "io", err: "맥에 쓰지 못했어요: \(error.localizedDescription)")
                        return
                    }
                    hasher.update(data: d)
                    pos += Int64(d.count)
                    if Date().timeIntervalSince(lastEmit) > 0.25 || pos >= size {
                        lastEmit = Date()
                        emit(["ev": "progress", "for": id, "done": pos])
                    }
                    step()
                }
            }
        }
        step()
    }

    func delete(_ id: Int, _ req: [String: Any]) {
        guard let c = camera else { reply(id, ok: false, code: "gone"); return }
        if isLocked { reply(id, ok: false, code: "locked"); return }
        let hs = (req["hs"] as? [String]) ?? []
        var items: [ICCameraItem] = []
        var want = Set<String>()
        for h in hs {
            if let f = files[h] {
                items.append(f)
                want.insert(h)
            }
        }
        if items.isEmpty { reply(id, ok: true, extra: ["deleted": [String]()]); return }
        say("지우기 부탁: \(items.count)개")
        deleting.append((id: id, want: want, got: [], last: Date()))
        c.requestDeleteFiles(items)
    }

    // 지운 항목은 didRemove 로 알려 와요. 다 오거나, 20초 동안 더 오지 않으면 대답해요.
    func checkDeletes() {
        var keep: [(id: Int, want: Set<String>, got: [String], last: Date)] = []
        for d in deleting {
            if Set(d.got) == d.want || Date().timeIntervalSince(d.last) > 20 || camera == nil {
                let ok = Set(d.got) == d.want
                reply(d.id, ok: ok, code: ok ? "" : failCode(), err: ok ? "" : "일부를 지우지 못했어요", extra: ["deleted": d.got])
                say("지움: \(d.got.count)/\(d.want.count)")
            } else {
                keep.append(d)
            }
        }
        deleting = keep
    }
}

func writeJPEG(_ img: CGImage, to path: String, max: Int) -> Bool {
    var out = img
    let w = img.width, h = img.height
    if w > max || h > max {
        let scale = Double(max) / Double(Swift.max(w, h))
        let nw = Int(Double(w) * scale), nh = Int(Double(h) * scale)
        if let ctx = CGContext(data: nil, width: nw, height: nh, bitsPerComponent: 8, bytesPerRow: 0,
                               space: CGColorSpace(name: CGColorSpace.sRGB)!, bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue) {
            ctx.interpolationQuality = .high
            ctx.draw(img, in: CGRect(x: 0, y: 0, width: nw, height: nh))
            if let s = ctx.makeImage() { out = s }
        }
    }
    let tmp = path + ".tmp"
    let url = URL(fileURLWithPath: tmp)
    guard let dest = CGImageDestinationCreateWithURL(url as CFURL, UTType.jpeg.identifier as CFString, 1, nil) else { return false }
    CGImageDestinationAddImage(dest, out, [kCGImageDestinationLossyCompressionQuality: 0.8] as CFDictionary)
    guard CGImageDestinationFinalize(dest) else { return false }
    return rename(tmp, path) == 0
}

// MARK: - 시작

let helper = Helper()
let parent = getppid()

// 엔진이 사라지면 함께 끝나요
Timer.scheduledTimer(withTimeInterval: 2, repeats: true) { _ in
    if getppid() != parent { say("엔진이 사라짐 → 끝"); exit(0) }
}

Thread.detachNewThread {
    while let line = readLine(strippingNewline: true) {
        guard let d = line.data(using: .utf8),
              let obj = try? JSONSerialization.jsonObject(with: d) as? [String: Any] else { continue }
        DispatchQueue.main.async { helper.run(obj) }
    }
    say("엔진과 연결이 끝남 → 끝")
    exit(0)
}

say("시작 · macOS \(ProcessInfo.processInfo.operatingSystemVersionString)")
helper.start()
RunLoop.main.run()
