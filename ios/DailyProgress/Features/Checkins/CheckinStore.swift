import Foundation
import Observation

// Stub - v1 checkin flow (step 5 of the build sequence)
@Observable
@MainActor
final class CheckinStore {
    var isLoading = false
    private let core: any CoreAPI

    init(core: any CoreAPI) {
        self.core = core
    }
}
