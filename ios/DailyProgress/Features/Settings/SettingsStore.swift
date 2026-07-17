import Foundation
import Observation

// Stub - step 7
@Observable
@MainActor
final class SettingsStore {
    var config: CoreConfig?
    var isLoading = false
    private let core: any CoreAPI

    init(core: any CoreAPI) {
        self.core = core
    }

    func load() async {
        isLoading = true
        defer { isLoading = false }
        do {
            let json = try await core.configJSON()
            config = try JSONDecoder().decode(CoreConfig.self, from: Data(json.utf8))
        } catch {
            // Non-fatal: defaults apply
        }
    }
}
