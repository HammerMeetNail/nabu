import Foundation

/// Volume unit helpers ported from the PWA's `web/static/js/utils.js`.
///
/// Volumes are stored canonically as milliliters (mL) everywhere in the DB and
/// API. The user's `volumeUnit` preference ("ml" | "oz") only changes how they
/// are displayed and how the log-sheet picker is labeled. US caregivers think
/// in ounces, so oz support is a display/input convenience over the same data.
enum VolumeUnits {
    static let mlPerOz = 29.5735

    static func mlToOz(_ ml: Int) -> Double {
        Double(ml) / mlPerOz
    }

    static func ozToMl(_ oz: Double) -> Int {
        Int((oz * mlPerOz).rounded())
    }

    /// "2.0" -> "2", "1.5" -> "1.5" — one decimal max, trailing .0 stripped.
    private static func trimNum(_ n: Double) -> String {
        let rounded = (n * 10).rounded() / 10
        if rounded == rounded.rounded() {
            return String(Int(rounded))
        }
        return String(format: "%.1f", rounded)
    }

    /// Renders a canonical mL amount in the user's preferred unit.
    static func formatVolume(_ ml: Int?, unit: String) -> String {
        guard let ml = ml else { return "" }
        if unit == "oz" {
            return "\(trimNum(mlToOz(ml))) oz"
        }
        return "\(ml) mL"
    }

    struct Option: Equatable {
        let ml: Int
        let label: String
    }

    /// Ordered list of choices for the log-sheet volume picker in the given
    /// unit. Option *values* are always canonical mL; only the labels differ
    /// by unit. `selectedML`, if it isn't already one of the presets, is
    /// appended so editing an existing log keeps its exact value selectable.
    static func volumeOptions(unit: String, selectedML: Int? = nil) -> [Option] {
        var opts: [Option] = []
        if unit == "oz" {
            for halfOz in 1...16 {
                let oz = Double(halfOz) * 0.5
                opts.append(Option(ml: ozToMl(oz), label: "\(trimNum(oz)) oz"))
            }
        } else {
            for ml in stride(from: 0, through: 200, by: 5) {
                opts.append(Option(ml: ml, label: "\(ml) mL"))
            }
        }
        if let selected = selectedML, !opts.contains(where: { $0.ml == selected }) {
            opts.append(Option(ml: selected, label: formatVolume(selected, unit: unit)))
            opts.sort { $0.ml < $1.ml }
        }
        return opts
    }
}

/// Amount columns keep their configured unit; the mL/oz preference applies
/// only to actual liquid-volume chores.
func formatChoreAmount(_ value: Int, chore: Chore?, volumeUnit: String) -> String {
    guard let chore, !["", "ml", "oz"].contains(chore.metricUnit.lowercased()) else {
        return VolumeUnits.formatVolume(value, unit: volumeUnit)
    }
    return "\(value) \(chore.metricUnit)"
}
