import XCTest
import Core
@testable import DailyProgress

// MARK: - CoreKit smoke tests

/// Step-1 smoke test: opens a Core against a temp dir, adds a task,
/// and reads TreeJSON back — proving the gomobile binding + decoding round-trips.
final class CoreKitTests: XCTestCase {

    // MARK: - Interop shape test

    /// Verifies that Core errors arrive as thrown NSErrors (not silent nil returns).
    /// Calls treeJSON with an invalid date and asserts it throws a CoreError.
    func testInvalidDateThrows() async throws {
        let tmpDir = FileManager.default.temporaryDirectory
            .appendingPathComponent("DPTest-\(UUID().uuidString)")
        defer { try? FileManager.default.removeItem(at: tmpDir) }

        let client = try CoreClient.open(
            dataDir: tmpDir.path,
            clientID: "",
            deviceID: "test-device"
        )

        do {
            _ = try await client.treeJSON(date: "not-a-date")
            XCTFail("Expected treeJSON to throw on invalid date")
        } catch CoreError.badInput(let msg) {
            // Expected: BAD_INPUT coded error for invalid date format
            XCTAssertFalse(msg.isEmpty, "Error detail should not be empty")
        } catch {
            XCTFail("Unexpected error type: \(error)")
        }
    }

    // MARK: - Round-trip test

    /// Adds a task and reads TreeJSON; asserts the task appears in unfiled tasks.
    func testAddTaskAndReadTree() async throws {
        let tmpDir = FileManager.default.temporaryDirectory
            .appendingPathComponent("DPTest-\(UUID().uuidString)")
        defer { try? FileManager.default.removeItem(at: tmpDir) }

        let client = try CoreClient.open(
            dataDir: tmpDir.path,
            clientID: "",
            deviceID: "test-device"
        )

        let today = Date().coreDate

        // Add a task
        try await client.addTask(date: today, text: "Test task from iOS unit test", projectID: "")

        // Read the tree back
        let json = try await client.treeJSON(date: today)
        XCTAssertFalse(json.isEmpty, "TreeJSON should not be empty")

        // Decode and verify
        let data = try XCTUnwrap(json.data(using: .utf8))
        let tree = try JSONDecoder().decode(ProjectTree.self, from: data)

        let allTasks = tree.unfiled + tree.projects.flatMap(\.tasks)
        XCTAssertTrue(
            allTasks.contains(where: { $0.text == "Test task from iOS unit test" }),
            "Added task should appear in the tree. Unfiled: \(tree.unfiled.map(\.text))"
        )
    }

    // MARK: - Error prefix test

    func testCASMismatchClassification() {
        let fakeError = NSError(
            domain: "go",
            code: 1,
            userInfo: [NSLocalizedDescriptionKey: "CAS_MISMATCH: tree is stale, please refresh"]
        )
        let classified = CoreError.classify(fakeError)
        guard case .casMismatch = classified else {
            XCTFail("Expected casMismatch, got \(classified)")
            return
        }
    }

    func testBadInputClassification() {
        let fakeError = NSError(
            domain: "go",
            code: 1,
            userInfo: [NSLocalizedDescriptionKey: "BAD_INPUT: invalid date \"foo\" (want YYYY-MM-DD)"]
        )
        let classified = CoreError.classify(fakeError)
        guard case .badInput(let msg) = classified else {
            XCTFail("Expected badInput, got \(classified)")
            return
        }
        XCTAssertTrue(msg.contains("invalid date"))
    }

    func testNotFoundClassification() {
        let fakeError = NSError(
            domain: "go",
            code: 1,
            userInfo: [NSLocalizedDescriptionKey: "NOT_FOUND: project abc not found"]
        )
        let classified = CoreError.classify(fakeError)
        guard case .notFound(let msg) = classified else {
            XCTFail("Expected notFound, got \(classified)")
            return
        }
        XCTAssertTrue(msg.contains("project abc"))
    }

    // MARK: - Model decoding tests

    func testProjectTreeDecoding() throws {
        let json = """
        {
            "projects": [
                {
                    "id": "proj1",
                    "name": "Work",
                    "done": false,
                    "tasks": [
                        {
                            "index": 0,
                            "depth": 0,
                            "text": "Finish report",
                            "state": "todo",
                            "date": "2026-07-17",
                            "done": false,
                            "children": []
                        }
                    ]
                }
            ],
            "unfiled": [],
            "recycled": [],
            "recurring": []
        }
        """
        let data = try XCTUnwrap(json.data(using: .utf8))
        let tree = try JSONDecoder().decode(ProjectTree.self, from: data)

        XCTAssertEqual(tree.projects.count, 1)
        XCTAssertEqual(tree.projects[0].name, "Work")
        XCTAssertEqual(tree.projects[0].tasks.count, 1)
        XCTAssertEqual(tree.projects[0].tasks[0].text, "Finish report")
        XCTAssertEqual(tree.projects[0].tasks[0].state, .todo)
        XCTAssertTrue(tree.unfiled.isEmpty)
    }

    func testItemStateDecodingFailsOnUnknown() throws {
        let json = """
        {
            "index": 0, "depth": 0, "text": "x", "state": "UNKNOWN",
            "date": "2026-07-17", "done": false, "children": []
        }
        """
        let data = try XCTUnwrap(json.data(using: .utf8))
        XCTAssertThrowsError(try JSONDecoder().decode(TreeTask.self, from: data)) { error in
            // Should be a decoding error, not a silent default
            XCTAssertTrue(error is DecodingError, "Expected DecodingError for unknown state")
        }
    }
}
