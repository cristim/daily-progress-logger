import SwiftUI

// Stub - step 5
struct EveningCheckinSheet: View {
    let appState: AppState
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            ContentUnavailableView(
                "Evening Check-in",
                systemImage: "moon.stars",
                description: Text("Coming in step 5.")
            )
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Dismiss") { dismiss() }
                }
            }
        }
    }
}
