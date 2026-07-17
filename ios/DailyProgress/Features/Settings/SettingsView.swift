import SwiftUI

// Stub - step 7
struct SettingsView: View {
    let appState: AppState

    var body: some View {
        ContentUnavailableView(
            "Settings",
            systemImage: "gearshape",
            description: Text("Coming in step 7.")
        )
        .navigationTitle("Settings")
    }
}
