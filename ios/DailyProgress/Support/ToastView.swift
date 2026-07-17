import SwiftUI

// MARK: - Toast view modifier

/// Applies a bottom-anchored ephemeral toast to any view.
/// Usage:
///   .toast(store.toast)
/// Extracted from TodayView so every screen can reuse it without copy-pasting
/// the overlay + material + transition + animation pattern (rule 11, phase A).
extension View {
    /// Shows a brief, non-blocking toast banner at the bottom of the view.
    /// Pass a non-nil String to display; nil hides the banner.
    func toast(_ message: String?) -> some View {
        modifier(ToastModifier(message: message))
    }
}

private struct ToastModifier: ViewModifier {
    let message: String?

    func body(content: Content) -> some View {
        content.overlay(alignment: .bottom) {
            if let message {
                Text(message)
                    .padding(.horizontal, 16)
                    .padding(.vertical, 10)
                    .background(.regularMaterial, in: Capsule())
                    .padding(.bottom, 24)
                    .transition(.move(edge: .bottom).combined(with: .opacity))
                    .animation(.easeInOut, value: message)
            }
        }
    }
}
