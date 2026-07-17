import SwiftUI

// Stub - step 7
struct RecurringView: View {
    let appState: AppState

    var body: some View {
        ContentUnavailableView(
            "Recurring Tasks",
            systemImage: "repeat",
            description: Text("Coming in step 7.")
        )
        .navigationTitle("Recurring")
    }
}
