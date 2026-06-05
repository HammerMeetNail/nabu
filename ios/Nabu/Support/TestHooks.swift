import Foundation

enum TestHooks {
    static var baseURLOverride: String? {
        let args = ProcessInfo.processInfo.arguments
        guard let index = args.firstIndex(of: "-nabuBaseURL"),
              index + 1 < args.count else {
            return nil
        }
        return args[index + 1]
    }

    static var resetState: Bool {
        ProcessInfo.processInfo.arguments.contains("-resetState")
    }

    static var disableAnimations: Bool {
        ProcessInfo.processInfo.arguments.contains("-disableAnimations")
    }

    static var useMockAPI: Bool {
        ProcessInfo.processInfo.arguments.contains("-useMockAPI")
    }
}
