import SwiftUI

// Stub - step 7
struct RecycleView: View {
    let appState: AppState

    var body: some View {
        ContentUnavailableView(
            "Recycle Bin",
            systemImage: "trash",
            description: Text("Coming in step 7.")
        )
        .navigationTitle("Recycle Bin")
    }
}
