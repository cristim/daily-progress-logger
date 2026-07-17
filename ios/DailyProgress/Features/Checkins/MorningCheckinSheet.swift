import SwiftUI

// Stub - step 5
struct MorningCheckinSheet: View {
    let appState: AppState
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            ContentUnavailableView(
                "Morning Check-in",
                systemImage: "sun.rise",
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
