import SwiftUI

// Stub - step 8
struct SyncView: View {
    let appState: AppState

    var body: some View {
        ContentUnavailableView(
            "Sync & Account",
            systemImage: "arrow.triangle.2.circlepath",
            description: Text("Google Drive sync coming in step 8.")
        )
        .navigationTitle("Sync & Account")
    }
}
