import SwiftUI

// Stub - step 6
struct WeekView: View {
    let appState: AppState

    var body: some View {
        NavigationStack {
            ContentUnavailableView(
                "Weekly View",
                systemImage: "calendar",
                description: Text("Coming in step 6.")
            )
            .navigationTitle("Week")
        }
    }
}
