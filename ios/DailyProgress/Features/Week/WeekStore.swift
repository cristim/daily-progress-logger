import Foundation
import Observation

// Stub - step 6
@Observable
@MainActor
final class WeekStore {
    var isLoading = false
    private let core: any CoreAPI

    init(core: any CoreAPI) {
        self.core = core
    }
}
