import SwiftUI

// Stub - step 7
struct BacklogView: View {
    let appState: AppState

    var body: some View {
        NavigationStack {
            ContentUnavailableView(
                "Backlog",
                systemImage: "tray.full",
                description: Text("Coming in step 7.")
            )
            .navigationTitle("Backlog")
        }
    }
}
