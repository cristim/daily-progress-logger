import Foundation
import Observation

// Stub - step 7
@Observable
@MainActor
final class ProjectsStore {
    var isLoading = false
    private let core: any CoreAPI

    init(core: any CoreAPI) {
        self.core = core
    }
}
