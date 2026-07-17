import SwiftUI

// Stub - step 7
struct ProjectsView: View {
    let appState: AppState

    var body: some View {
        ContentUnavailableView(
            "Projects",
            systemImage: "folder",
            description: Text("Coming in step 7.")
        )
        .navigationTitle("Projects")
    }
}
