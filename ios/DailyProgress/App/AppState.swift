import Foundation
import Observation
import UIKit

// MARK: - AppState

/// Root observable state shared by all feature stores.
/// The single CoreClient is opened here at launch and injected down the tree.
@Observable
@MainActor
final class AppState {
    /// The opened core; nil before launch completes or if open failed.
    var core: (any CoreAPI)?
    /// Currently viewed date in the Today tab.
    var viewedDate: Date = Date()
    /// Bumped after every mutation or sync so sibling screens know to refresh.
    var dataVersion: Int = 0
    /// Non-nil when app-level setup failed.
    var launchError: String?
    /// Prompts due at launch/foreground; drives check-in routing.
    var duePrompts: [DuePrompt] = []

    /// Opens the Go core. dataDir is the app sandbox Documents/DailyProgress.
    func openCore() async {
        let docs = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0]
        let dataDir = docs.appendingPathComponent("DailyProgress", isDirectory: true).path

        // TODO(step 8): clientID should come from persisted config, not UserDefaults
        let clientID = loadClientID()
        let deviceID = loadDeviceID()

        do {
            // CoreClient.open does file I/O and a migration pass; run it on a
            // background thread so the main thread is never blocked at launch.
            let client = try await Task.detached(priority: .userInitiated) {
                try CoreClient.open(dataDir: dataDir, clientID: clientID, deviceID: deviceID)
            }.value
            core = client
        } catch {
            launchError = error.localizedDescription
        }
    }

    /// Bumps the data version so all visible screens re-fetch.
    func bumpDataVersion() {
        dataVersion += 1
    }

    /// Evaluates which prompts are due right now and updates duePrompts.
    /// Uses CoreDecoding so decode failures surface as contractViolation (not swallowed).
    /// Non-fatal: a DuePromptsJSON error is advisory and should not block the UI.
    func refreshDuePrompts() async {
        guard let core else { return }
        do {
            let json = try await core.duePromptsJSON(nowRFC3339: Date().rfc3339)
            let result = try CoreDecoding.decode(DuePrompts.self, from: json)
            duePrompts = result.due
        } catch {
            // Non-fatal: prompts are advisory; log but do not surface to the user.
        }
    }

    // MARK: Private helpers

    private func loadClientID() -> String {
        // Loaded from persisted config if available; empty until user sets up sync.
        UserDefaults.standard.string(forKey: "google_client_id") ?? ""
    }

    private func loadDeviceID() -> String {
        let key = "device_id"
        if let existing = UserDefaults.standard.string(forKey: key) {
            return existing
        }
        let vendorID = UIDevice.current.identifierForVendor?.uuidString ?? UUID().uuidString
        let deviceID = "ios-\(vendorID)"
        UserDefaults.standard.set(deviceID, forKey: key)
        return deviceID
    }
}
