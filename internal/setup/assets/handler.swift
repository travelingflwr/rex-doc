// rdxt's document handler: the app macOS opens .md and .dok files (and .txt,
// after rdxt setup --txt) with
// (Cmd-click in Rex, double-click in Finder). It hands each file to
// `rdxt open --click`, which shows it in Rex where `rdxt setup --open` said
// (a floating window, or a pane beside the focused one), and quits.
// Built and installed by `rdxt setup`.
import AppKit
import CoreServices

let rdxt = "__RDXT__"
let rexBin = "__REX__"
let bundleID = "com.travelingflwr.rdxt"
let types = [__TYPES__]

func setDefault() {
    var previous: [String: String] = [:]
    for t in types {
        if let h = LSCopyDefaultRoleHandlerForContentType(t as CFString, .all)?.takeRetainedValue() as String?,
           h.lowercased() != bundleID {
            previous[t] = h
        }
        LSSetDefaultRoleHandlerForContentType(t as CFString, .all, bundleID as CFString)
    }
    let data = try! JSONSerialization.data(withJSONObject: previous)
    print(String(data: data, encoding: .utf8)!)
}

func restore(_ pairs: ArraySlice<String>) {
    for pair in pairs {
        let parts = pair.split(separator: "=", maxSplits: 1).map(String.init)
        if parts.count == 2 {
            LSSetDefaultRoleHandlerForContentType(parts[0] as CFString, .all, parts[1] as CFString)
        }
    }
}

func open(_ path: String) {
    var env = ProcessInfo.processInfo.environment
    env["RDXT_REX"] = rexBin
    env["PATH"] = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:" + (env["PATH"] ?? "")
    if run(rdxt, ["open", "--click", path], env) == 0 {
        bringRexForward()
        return
    }
    // Rex isn't running: start it, then try once more.
    bringRexForward()
    Thread.sleep(forTimeInterval: 3)
    _ = run(rdxt, ["open", "--click", path], env)
}

// run starts a program and waits for it, returning its exit status (-1 if it
// couldn't start). The program is its own responsible process, as in a
// terminal: when the rex command it runs updates Rex Server, macOS treats that
// as Rex's doing, and doesn't ask whether rdxt.app may manage other apps.
func run(_ exe: String, _ args: [String], _ env: [String: String]) -> Int32 {
    typealias Disclaim = @convention(c) (UnsafeMutablePointer<posix_spawnattr_t?>, Int32) -> Int32
    var attr: posix_spawnattr_t? = nil
    posix_spawnattr_init(&attr)
    defer { posix_spawnattr_destroy(&attr) }
    if let sym = dlsym(UnsafeMutableRawPointer(bitPattern: -2), "responsibility_spawnattrs_setdisclaim") {
        _ = unsafeBitCast(sym, to: Disclaim.self)(&attr, 1)
    }
    let argv = ([exe] + args).map { strdup($0) } + [nil]
    let envp = env.map { strdup("\($0.key)=\($0.value)") } + [nil]
    defer { (argv + envp).forEach { free($0) } }
    var pid: pid_t = 0
    guard posix_spawn(&pid, exe, nil, &attr, argv, envp) == 0 else { return -1 }
    var status: Int32 = 0
    while waitpid(pid, &status, 0) == -1 && errno == EINTR {}
    return (status & 0x7f) == 0 ? (status >> 8) & 0xff : -1
}

// bringRexForward starts Rex, or brings it to the front when it's running,
// so a document opened from Finder is in front of you rather than behind it.
func bringRexForward() {
    if let rexApp = NSWorkspace.shared.urlForApplication(withBundleIdentifier: "com.superlogical.rex") {
        NSWorkspace.shared.open(rexApp)
    }
}

let args = CommandLine.arguments
if args.count > 1 && args[1] == "--set-default" {
    setDefault()
    exit(0)
}
if args.count > 1 && args[1] == "--restore" {
    restore(args.dropFirst(2))
    exit(0)
}

class Delegate: NSObject, NSApplicationDelegate {
    func application(_ app: NSApplication, open urls: [URL]) {
        for u in urls where u.isFileURL { open(u.path) }
        NSApp.terminate(nil)
    }
    func applicationDidFinishLaunching(_ n: Notification) {
        // Launched without a file (from Finder or Spotlight): nothing to do.
        DispatchQueue.main.asyncAfter(deadline: .now() + 5) { NSApp.terminate(nil) }
    }
}

let app = NSApplication.shared
let delegate = Delegate()
app.delegate = delegate
app.setActivationPolicy(.prohibited)
app.run()
