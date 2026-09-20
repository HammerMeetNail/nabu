import XCTest
@testable import Nabu

/// Round-trips the mL/oz conversion helpers against the PWA's
/// `utils.js` cases (`tests/unit` volume specs) so both clients render
/// identical labels for the same canonical mL values.
final class VolumeUnitsTests: XCTestCase {

    func testActivityNumberChoicesMatchPWARangesAndUnits() {
        let ml = ActivityNumberChoices.options(unit: "mL")
        XCTAssertEqual(ml.count, 90) // blank + 0...300 by 5 + 325...1000 by 25
        XCTAssertEqual(ml.first, .init(value: "", label: "No value"))
        XCTAssertEqual(ml[1], .init(value: "0", label: "0 mL"))
        XCTAssertEqual(ml.last, .init(value: "1000", label: "1000 mL"))
        let oz = ActivityNumberChoices.options(unit: "oz")
        XCTAssertEqual(oz.count, 34)
        XCTAssertTrue(oz.contains(.init(value: "2.5", label: "2.5 oz")))
        for unit in AmountUnits.common + ["reps", "<custom>"] where !["mL", "oz"].contains(unit) {
            let choices = ActivityNumberChoices.options(unit: unit)
            XCTAssertEqual(choices.count, 74)
            XCTAssertTrue(choices.contains(.init(value: "2", label: "2 \(unit)")))
        }
    }

    func testActivityNumberChoicesPreserveExactValuesAndCanonicalConversion() {
        for (unit, stored) in [("mL", 12345), ("oz", 37), ("mg", 0)] {
            let text = AmountUnits.inputText(stored, unit: unit)
            let selected = ActivityNumberChoices.selection(for: text, unit: unit)
            let choices = ActivityNumberChoices.options(unit: unit, currentText: text)
            XCTAssertTrue(choices.contains { $0.value == selected })
            XCTAssertEqual(AmountUnits.storedAmount(selected, unit: unit), stored)
        }
        XCTAssertEqual(ActivityNumberChoices.selection(for: "2,5", unit: "oz"), "2.5")
        for value in ["", "-1", "nan", "inf", "100001"] {
            XCTAssertEqual(ActivityNumberChoices.selection(for: value, unit: "mg"), "")
        }
        XCTAssertEqual(ActivityNumberChoices.selection(for: "4000", unit: "oz"), "")
    }

    func testActivityDurationChoicesPreserveExactSecondsAndBounds() {
        let choices = ActivityNumberChoices.options(kind: .duration, currentText: "301")
        XCTAssertEqual(choices.count, 89) // blank, 87 presets, exact custom value
        XCTAssertTrue(choices.contains(.init(value: "300", label: "5 min")))
        XCTAssertTrue(choices.contains(.init(value: "301", label: "301 sec")))
        XCTAssertEqual(choices.last, .init(value: "86400", label: "24 hr"))
        XCTAssertTrue(choices.allSatisfy { $0.value.isEmpty || Double($0.value)! <= 86400 })
        XCTAssertEqual(ActivityNumberChoices.selection(for: "86401", kind: .duration), "")
    }

    func testIndependentAmountTextAndStoredUnits() {
        for unit in ["mcg", "mg", "g", "mL", "L", "drops", "tablets", "capsules", "puffs", "units"] {
            XCTAssertEqual(AmountUnits.inputText(200, unit: unit), "200")
            XCTAssertEqual(AmountUnits.storedAmount("200", unit: unit), 200)
        }
        XCTAssertEqual(AmountUnits.storedAmount("4", unit: "oz"), 118)
        XCTAssertEqual(AmountUnits.storedAmount(AmountUnits.inputText(321, unit: "oz"), unit: "oz"), 321)
        XCTAssertNil(AmountUnits.storedAmount("", unit: "mg"))
        XCTAssertNil(AmountUnits.storedAmount("-1", unit: "mg"))
    }

    func testCustomAmountUnitsIgnoreLiquidVolumePreference() {
        let chore = Chore(id: 1, householdId: 1, name: "Cat meds", icon: "💊",
                          color: "#A78BFA", sortOrder: 0, category: "", isPredefined: false,
                          predefinedKey: nil, createdBy: nil, createdAt: Date(),
                          indicatorLabels: [], indicatorDefaults: [], hasVolumeML: true, metricType: "amount", metricUnit: "mg")
        XCTAssertEqual(formatChoreAmount(25, chore: chore, volumeUnit: "ml"), "25 mg")
        XCTAssertEqual(formatChoreAmount(25, chore: chore, volumeUnit: "oz"), "25 mg")
    }

    // MARK: - Conversion

    func testOzToMlMatchesJS() {
        XCTAssertEqual(VolumeUnits.ozToMl(0.5), 15)
        XCTAssertEqual(VolumeUnits.ozToMl(1), 30)
        XCTAssertEqual(VolumeUnits.ozToMl(2.5), 74)
        XCTAssertEqual(VolumeUnits.ozToMl(4), 118)
        XCTAssertEqual(VolumeUnits.ozToMl(8), 237)
    }

    func testMlToOz() {
        XCTAssertEqual(VolumeUnits.mlToOz(120), 120 / 29.5735, accuracy: 0.0001)
        XCTAssertEqual(VolumeUnits.mlToOz(0), 0)
    }

    // MARK: - formatVolume

    func testFormatVolumeML() {
        XCTAssertEqual(VolumeUnits.formatVolume(120, unit: "ml"), "120 mL")
        XCTAssertEqual(VolumeUnits.formatVolume(0, unit: "ml"), "0 mL")
        XCTAssertEqual(VolumeUnits.formatVolume(nil, unit: "ml"), "")
    }

    func testFormatVolumeOz() {
        XCTAssertEqual(VolumeUnits.formatVolume(120, unit: "oz"), "4.1 oz")
        XCTAssertEqual(VolumeUnits.formatVolume(30, unit: "oz"), "1 oz")   // 1.0 -> "1"
        XCTAssertEqual(VolumeUnits.formatVolume(15, unit: "oz"), "0.5 oz")
        XCTAssertEqual(VolumeUnits.formatVolume(nil, unit: "oz"), "")
    }

    // MARK: - volumeOptions

    func testVolumeOptionsML() {
        let opts = VolumeUnits.volumeOptions(unit: "ml")
        XCTAssertEqual(opts.count, 41) // 0...200 step 5
        XCTAssertEqual(opts.first, VolumeUnits.Option(ml: 0, label: "0 mL"))
        XCTAssertEqual(opts.last, VolumeUnits.Option(ml: 200, label: "200 mL"))
    }

    func testVolumeOptionsOz() {
        let opts = VolumeUnits.volumeOptions(unit: "oz")
        XCTAssertEqual(opts.count, 16) // 0.5...8 step 0.5
        XCTAssertEqual(opts.first, VolumeUnits.Option(ml: 15, label: "0.5 oz"))
        XCTAssertEqual(opts.last, VolumeUnits.Option(ml: 237, label: "8 oz"))
        // Values are always canonical mL, only labels differ.
        XCTAssertTrue(opts.allSatisfy { $0.label.hasSuffix(" oz") })
    }

    func testVolumeOptionsAppendsNonPresetSelection() {
        let opts = VolumeUnits.volumeOptions(unit: "ml", selectedML: 63)
        XCTAssertEqual(opts.count, 42)
        // Kept sorted, so 63 sits between 60 and 65.
        let idx = opts.firstIndex(where: { $0.ml == 63 })
        XCTAssertNotNil(idx)
        XCTAssertEqual(opts[idx! - 1].ml, 60)
        XCTAssertEqual(opts[idx! + 1].ml, 65)
        XCTAssertEqual(opts[idx!].label, "63 mL")
    }

    func testVolumeOptionsDoesNotDuplicatePresetSelection() {
        // 118 mL == 4 oz, already a preset in oz mode.
        let opts = VolumeUnits.volumeOptions(unit: "oz", selectedML: 118)
        XCTAssertEqual(opts.count, 16)
    }

    func testOzRoundTripAcrossAllPresets() {
        // Every oz preset must survive mL canonical storage and format back
        // to its own label (the JS round-trip cases).
        for halfOz in 1...16 {
            let oz = Double(halfOz) * 0.5
            let ml = VolumeUnits.ozToMl(oz)
            let label = VolumeUnits.formatVolume(ml, unit: "oz")
            let expected = oz == oz.rounded() ? "\(Int(oz)) oz" : String(format: "%.1f oz", oz)
            XCTAssertEqual(label, expected, "round-trip failed for \(oz) oz (\(ml) mL)")
        }
    }
}
