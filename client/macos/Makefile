.PHONY: build install startup dmg notarize release clean

VERSION := $(shell git describe --tags --match 'v*' 2>/dev/null | sed 's/^v//' || echo "0.0.0-dev")
DMG := Yucca-$(VERSION).dmg

# Developer ID Application identity (the cert's private key must be in the keychain).
SIGN_ID := Developer ID Application: Marek Kobylinski (6QLFSTUS7B)
# notarytool keychain profile created with: xcrun notarytool store-credentials
NOTARY_PROFILE := yucca-notary

build:
	swiftc -O -target arm64-apple-macosx13.0 -o Yucca main.swift
	strip Yucca
	mkdir -p Yucca.app/Contents/MacOS Yucca.app/Contents/Resources
	cp Yucca Yucca.app/Contents/MacOS/
	sed 's/__VERSION__/$(VERSION)/g' Info.plist > Yucca.app/Contents/Info.plist
	cp AppIcon.icns Yucca.app/Contents/Resources/
	codesign --force --sign - Yucca.app

install: build
	@if pgrep -x Yucca >/dev/null; then killall Yucca; sleep 1; fi
	rm -rf /Applications/Yucca.app
	cp -r Yucca.app /Applications/Yucca.app
	open /Applications/Yucca.app

startup:
	@echo "Adding Yucca to login items..."
	osascript -e 'tell application "System Events" to make login item at end with properties {path:"/Applications/Yucca.app", hidden:false}'
	@echo "Yucca will start at login."

dmg: build
	codesign --force --options runtime --timestamp --sign "$(SIGN_ID)" Yucca.app
	rm -rf dmg-stage $(DMG)
	mkdir -p dmg-stage
	cp -r Yucca.app dmg-stage/
	ln -s /Applications dmg-stage/Applications
	hdiutil create -volname "Yucca" -srcfolder dmg-stage -ov -format UDZO $(DMG)
	rm -rf dmg-stage

notarize: dmg
	@echo "Notarizing $(DMG) with Apple (may take a few minutes)..."
	xcrun notarytool submit $(DMG) --keychain-profile "$(NOTARY_PROFILE)" --wait
	xcrun stapler staple $(DMG)

release: notarize
	@if [ "$(VERSION)" = "0.0.0-dev" ]; then echo "Error: no git tag found. Run: git tag v0.1.0" && exit 1; fi
	@echo "Creating release v$(VERSION) with $(DMG)"
	gh release create "v$(VERSION)" $(DMG) --notes "Yucca macOS client v$(VERSION)"

clean:
	rm -f Yucca
	rm -rf Yucca.app
	rm -f Yucca-*.dmg
	rm -rf dmg-stage
