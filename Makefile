# Daily Progress Logger — Go + Qt (miqt) macOS desktop app.
#
# The miqt Qt bindings compile a large cgo surface; the first build takes
# several minutes, later builds hit the cache. Qt headers require C++17+,
# which Apple clang does not default to, hence CGO_CXXFLAGS.

APP_NAME    := DailyProgressLogger
BINARY      := daily-progress-logger
BUNDLE_ID   := com.cristim.daily-progress-logger
BUILD_DIR   := build
APP_BUNDLE  := $(BUILD_DIR)/$(APP_NAME).app
PLIST       := $(HOME)/Library/LaunchAgents/$(BUNDLE_ID).plist
MACDEPLOYQT := /opt/homebrew/opt/qt/bin/macdeployqt

export CGO_CXXFLAGS := -std=c++20

.PHONY: build test lint run screenshot app install-agent uninstall-agent clean

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/$(BINARY)

test:
	go test -race -cover ./...

lint:
	golangci-lint run

run: build
	$(BUILD_DIR)/$(BINARY)

screenshot: build
	mkdir -p $(BUILD_DIR)/screenshots
	$(BUILD_DIR)/$(BINARY) -screenshot $(BUILD_DIR)/screenshots

# Assemble a .app bundle and vendor the Qt frameworks into it with
# macdeployqt, so the app survives Homebrew Qt upgrades.
app: build
	rm -rf $(APP_BUNDLE)
	mkdir -p $(APP_BUNDLE)/Contents/MacOS
	cp $(BUILD_DIR)/$(BINARY) $(APP_BUNDLE)/Contents/MacOS/$(BINARY)
	sed -e 's/@BINARY@/$(BINARY)/g' -e 's/@BUNDLE_ID@/$(BUNDLE_ID)/g' \
		-e 's/@APP_NAME@/$(APP_NAME)/g' \
		packaging/Info.plist.template > $(APP_BUNDLE)/Contents/Info.plist
	$(MACDEPLOYQT) $(APP_BUNDLE) -executable=$(APP_BUNDLE)/Contents/MacOS/$(BINARY)

# Install a LaunchAgent so the app starts (hidden in the menu bar) at login.
install-agent: app
	mkdir -p $(HOME)/Library/LaunchAgents
	sed -e 's|@EXECUTABLE@|$(abspath $(APP_BUNDLE))/Contents/MacOS/$(BINARY)|g' \
		-e 's/@BUNDLE_ID@/$(BUNDLE_ID)/g' \
		packaging/launchagent.plist.template > $(PLIST)
	launchctl unload $(PLIST) 2>/dev/null || true
	launchctl load $(PLIST)
	@echo "LaunchAgent installed: $(PLIST)"

uninstall-agent:
	launchctl unload $(PLIST) 2>/dev/null || true
	rm -f $(PLIST)
	@echo "LaunchAgent removed"

clean:
	rm -rf $(BUILD_DIR)
