import Foundation
import Observation

// Stub - step 8
@Observable
@MainActor
final class SyncStore {
    var isLoading = false
    var isSignedIn = false
    private let core: any CoreAPI

    init(core: any CoreAPI) {
        self.core = core
    }
}
