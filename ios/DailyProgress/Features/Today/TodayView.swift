import SwiftUI

// MARK: - TodayView

struct TodayView: View {
    let appState: AppState
    let coordinator: CheckinCoordinator
    @State private var store: TodayStore

    @State private var showAddTask = false
    @State private var showDatePicker = false

    @Environment(\.showMorningCheckin) private var showMorningCheckin
    @Environment(\.showEveningCheckin) private var showEveningCheckin

    init(appState: AppState, coordinator: CheckinCoordinator) {
        self.appState = appState
        self.coordinator = coordinator
        let core = appState.core!
        _store = State(initialValue: TodayStore(core: core))
    }

    var body: some View {
        NavigationStack {
            Group {
                if store.isLoading && store.tree == nil {
                    ProgressView("Loading…")
                        .frame(maxWidth: .infinity, maxHeight: .infinity)
                } else if let tree = store.tree {
                    treeContent(tree: tree)
                } else if let error = store.errorMessage {
                    ContentUnavailableView(
                        "Failed to Load",
                        systemImage: "exclamationmark.triangle",
                        description: Text(error)
                    )
                } else {
                    ContentUnavailableView(
                        "No Tasks",
                        systemImage: "checkmark.circle",
                        description: Text("Tap + to add a task for today.")
                    )
                }
            }
            .navigationTitle(navigationTitle)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .principal) {
                    dateNavigation
                }
                ToolbarItem(placement: .primaryAction) {
                    HStack {
                        // Check-in menu (manual triggers)
                        Menu {
                            Button {
                                showMorningCheckin()
                            } label: {
                                Label("Morning Check-in...", systemImage: "sun.rise")
                            }
                            Button {
                                showEveningCheckin()
                            } label: {
                                Label("Evening Check-in...", systemImage: "moon.stars")
                            }
                        } label: {
                            Image(systemName: "ellipsis.circle")
                        }

                        Button {
                            showAddTask = true
                        } label: {
                            Image(systemName: "plus")
                        }
                    }
                }
            }
            .sheet(isPresented: $showAddTask) {
                AddTaskSheet(date: appState.viewedDate, store: store, appState: appState)
            }
            .toast(store.toast)
            // Show mutation/refresh errors as an alert when the tree is already
            // loaded; when tree == nil the error is rendered inline by the Group above.
            .alert("Error", isPresented: Binding(
                get: { store.tree != nil && store.errorMessage != nil },
                set: { if !$0 { store.errorMessage = nil } }
            )) {
                Button("OK", role: .cancel) { store.errorMessage = nil }
            } message: {
                if let msg = store.errorMessage {
                    Text(msg)
                }
            }
        }
        .task(id: appState.viewedDate) {
            await store.refresh(date: appState.viewedDate)
        }
        .task(id: appState.dataVersion) {
            // Refresh when another tab mutates shared data.
            if appState.dataVersion > 0 {
                await store.refresh(date: appState.viewedDate)
            }
        }
    }

    // MARK: - Tree content

    @ViewBuilder
    private func treeContent(tree: ProjectTree) -> some View {
        List {
            // Projects
            ForEach(tree.projects) { project in
                Section {
                    ForEach(project.tasks) { task in
                        taskRowWithChildren(task: task)
                    }
                } header: {
                    HStack {
                        Text(project.name)
                            .font(.headline)
                            .strikethrough(project.done)
                        Spacer()
                    }
                }
            }

            // Unfiled tasks
            if !tree.unfiled.isEmpty {
                Section("Unfiled") {
                    ForEach(tree.unfiled) { task in
                        taskRowWithChildren(task: task)
                    }
                }
            }

            // Recurring templates (collapsed summary)
            if !tree.recurring.isEmpty {
                Section {
                    DisclosureGroup("Recurring (\(tree.recurring.count))") {
                        ForEach(tree.recurring) { template in
                            HStack {
                                Image(systemName: "repeat")
                                    .foregroundStyle(.secondary)
                                    .frame(width: 20)
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(template.text)
                                    if let desc = template.describe {
                                        Text(desc)
                                            .font(.caption)
                                            .foregroundStyle(.secondary)
                                    }
                                }
                            }
                        }
                    }
                }
            }

            // Recycled items (collapsed link)
            if !tree.recycled.isEmpty {
                Section {
                    NavigationLink("Recycle Bin (\(tree.recycled.count) items)") {
                        // Placeholder - RecycleView wired in MoreView
                        Text("Recycle Bin")
                    }
                }
            }
        }
        .listStyle(.insetGrouped)
        .refreshable {
            await store.refresh(date: appState.viewedDate)
        }
    }

    // AnyView is required here because the function is recursive;
    // SwiftUI's opaque return type cannot express self-referential types.
    private func taskRowWithChildren(task: TreeTask) -> AnyView {
        AnyView(
            Group {
                TaskRow(task: task, store: store, appState: appState)
                ForEach(task.children) { child in
                    self.taskRowWithChildren(task: child)
                }
            }
        )
    }

    // MARK: - Date navigation

    private var navigationTitle: String {
        let cal = Calendar.current
        if cal.isDateInToday(appState.viewedDate) { return "Today" }
        if cal.isDateInYesterday(appState.viewedDate) { return "Yesterday" }
        if cal.isDateInTomorrow(appState.viewedDate) { return "Tomorrow" }
        return DateFormatting.string(from: appState.viewedDate)
    }

    private var dateNavigation: some View {
        HStack(spacing: 16) {
            Button {
                appState.viewedDate = Calendar.current.date(
                    byAdding: .day, value: -1, to: appState.viewedDate) ?? appState.viewedDate
            } label: {
                Image(systemName: "chevron.left")
            }

            Button {
                showDatePicker.toggle()
            } label: {
                Text(navigationTitle)
                    .font(.headline)
            }
            .popover(isPresented: $showDatePicker) {
                DatePicker(
                    "Select Date",
                    selection: Binding(
                        get: { appState.viewedDate },
                        set: { appState.viewedDate = $0 }
                    ),
                    displayedComponents: .date
                )
                .datePickerStyle(.graphical)
                .padding()
                .presentationDetents([.height(380)])
            }

            Button {
                appState.viewedDate = Calendar.current.date(
                    byAdding: .day, value: 1, to: appState.viewedDate) ?? appState.viewedDate
            } label: {
                Image(systemName: "chevron.right")
            }

            if !Calendar.current.isDateInToday(appState.viewedDate) {
                Button("Today") {
                    appState.viewedDate = Date()
                }
                .font(.subheadline)
            }
        }
    }
}
