import XCTest
import SwiftUI
@testable import Nabu

/// The log sheet's stars, like the PWA's star-rating slider: a tap is measured
/// across the stars themselves, and they grow with Dynamic Type instead of
/// wrapping out of a fixed frame.
@MainActor
final class StarRatingTests: XCTestCase {
    func testATapMapsAcrossTheStarsToTheNearestHalfStar() {
        XCTAssertEqual(StarRatingView.rating(at: 0, of: 110), 0)
        XCTAssertEqual(StarRatingView.rating(at: 55, of: 110), 25)
        XCTAssertEqual(StarRatingView.rating(at: 100, of: 110), 45, "the last star's middle")
        XCTAssertEqual(StarRatingView.rating(at: 106, of: 110), 50, "the last star's right edge")
        XCTAssertEqual(StarRatingView.rating(at: -8, of: 110), 0, "a drag past the first star")
        XCTAssertEqual(StarRatingView.rating(at: 150, of: 110), 50, "a drag past the last star")
    }

    func testTheViewIsTheStarsOwnSizeAtEveryTextSize() {
        func width<V: View>(_ view: V, at size: DynamicTypeSize) -> CGFloat {
            UIHostingController(rootView: view.environment(\.dynamicTypeSize, size))
                .sizeThatFits(in: CGSize(width: 2000, height: 2000)).width
        }
        let stars = StarRatingView(rating: .constant(25))
        for size in [DynamicTypeSize.large, .accessibility1] {
            let text = max(width(Text("☆☆☆☆☆").font(.title2), at: size),
                           width(Text("★★★★★").font(.title2), at: size))
            XCTAssertEqual(width(stars, at: size), text, accuracy: 0.5, "a tap is measured across the stars at \(size)")
        }
        XCTAssertGreaterThan(width(stars, at: .accessibility1), width(stars, at: .large) * 1.4,
                             "the stars grow with the text")
    }
}
