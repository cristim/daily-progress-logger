import SwiftUI

// MARK: - RecurringView

/// Recurring templates screen (More > Recurring Tasks): list, add, and remove
/// recurring task templates. Sourced from TreeJSON(today).recurring — see
/// RecurringStore's doc comment for why (it carries `describe`, RecurringJSON
/// does not). Removing a template does not remove tasks already materialized
/// from it into a day's plan.
struct RecurringView: View {
    let appState: AppState
    @State private var store: RecurringStore
    @State private var showingAdd = false
    @State private var pendingDelete: RecurringTemplate?

    init(appState: AppState) {
        self.appState = appState
        // core is guaranteed non-nil when RecurringView initialises: DailyProgressApp only
        // renders RootTabView (and thus tab views) after openCore() succeeds.
        _store = State(initialValue: RecurringStore(
            core: appState.core!,
            onMutation: { appState.bumpDataVersion() }
        ))
    }

    var body: some View {
        loadedContent
            .navigationTitle("Recurring Tasks")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar { addToolbarItem }
            .sheet(isPresented: $showingAdd) {
                AddRecurringSheet(store: store)
            }
            .modifier(RecurringErrorAlert(store: store))
            .modifier(DeleteRecurringConfirmation(store: store, pendingDelete: $pendingDelete))
            // Refresh when another tab mutates shared data (bumpDataVersion).
            .task(id: appState.dataVersion) {
                await store.refresh()
            }
    }

    // MARK: - Top-level content (loading / error / loaded)

    @ViewBuilder
    private var loadedContent: some View {
        if store.isLoading && store.templates == nil {
            ProgressView("Loading\u{2026}")
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else if store.templates == nil, let errMsg = store.errorMessage {
            ContentUnavailableView(
                "Failed to Load",
                systemImage: "exclamationmark.triangle",
                description: Text(errMsg)
            )
        } else {
            recurringContent
        }
    }

    private var addToolbarItem: some ToolbarContent {
        ToolbarItem(placement: .primaryAction) {
            Button {
                showingAdd = true
            } label: {
                Label("Add Recurring Task", systemImage: "plus")
            }
        }
    }

    // MARK: - Content

    private var recurringContent: some View {
        List {
            ForEach(store.templates ?? []) { template in
                recurringRow(template)
            }
        }
        .listStyle(.insetGrouped)
        .refreshable { await store.refresh() }
        .overlay {
            if let templates = store.templates, templates.isEmpty {
                ContentUnavailableView(
                    "No Recurring Tasks",
                    systemImage: "repeat",
                    description: Text("Add a task with a recurrence tag like @daily.")
                )
                .allowsHitTesting(false)
            }
        }
    }

    // MARK: - Row

    @ViewBuilder
    private func recurringRow(_ template: RecurringTemplate) -> some View {
        HStack(alignment: .top, spacing: 12) {
            Image(systemName: "repeat")
                .foregroundStyle(.secondary)
                .padding(.top, 2)
            VStack(alignment: .leading, spacing: 2) {
                Text(template.text)
                // TreeJSON always fills describe; RecurringJSON does not — this
                // screen reads TreeJSON, but nil is handled defensively (no caption
                // rather than an empty one, per plan §6).
                if let describe = template.describe, !describe.isEmpty {
                    Text(describe)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                if !template.project.isEmpty {
                    Text(template.project)
                        .font(.caption)
                        .foregroundStyle(.tertiary)
                }
            }
        }
        .swipeActions(edge: .trailing) {
            Button(role: .destructive) {
                pendingDelete = template
            } label: {
                Label("Delete", systemImage: "trash")
            }
        }
        .contextMenu {
            Button(role: .destructive) {
                pendingDelete = template
            } label: {
                Label("Delete", systemImage: "trash")
            }
        }
    }
}

// MARK: - RecurringErrorAlert

/// Shows mutation/refresh errors as an alert when content is already visible (rule 4).
/// Extracted as a ViewModifier (rather than chained inline in `body`) to keep the
/// main modifier chain small enough for the type checker to infer quickly.
private struct RecurringErrorAlert: ViewModifier {
    let store: RecurringStore

    func body(content: Content) -> some View {
        content.alert("Error", isPresented: Binding(
            get: { store.templates != nil && store.errorMessage != nil },
            set: { if !$0 { store.errorMessage = nil } }
        )) {
            Button("OK", role: .cancel) { store.errorMessage = nil }
        } message: {
            if let msg = store.errorMessage { Text(msg) }
        }
    }
}

// MARK: - DeleteRecurringConfirmation

/// Confirms an irrecoverable template deletion (plan rule 10). Extracted as a
/// ViewModifier for the same type-checker reason as RecurringErrorAlert.
private struct DeleteRecurringConfirmation: ViewModifier {
    let store: RecurringStore
    @Binding var pendingDelete: RecurringTemplate?

    func body(content: Content) -> some View {
        content.confirmationDialog(
            "Delete this recurring task? Already-created occurrences stay.",
            isPresented: Binding(
                get: { pendingDelete != nil },
                set: { if !$0 { pendingDelete = nil } }
            ),
            titleVisibility: .visible
        ) {
            Button("Delete", role: .destructive) {
                if let template = pendingDelete {
                    Task { await store.remove(raw: template.raw) }
                }
                pendingDelete = nil
            }
            Button("Cancel", role: .cancel) { pendingDelete = nil }
        }
    }
}
