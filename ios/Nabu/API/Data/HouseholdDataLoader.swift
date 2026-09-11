import Foundation

@MainActor
final class HouseholdDataLoader {
    let api: APIClient
    let state: AppState

    init(api: APIClient, state: AppState) {
        self.api = api
        self.state = state
    }

    func loadHouseholdData() async {
        let api = self.api.scoped()
        let owner = state.beginOperation("loadHouseholdData")
        do {
            let (data, listData) = try await (
                api.get("/api/household") as HouseholdResponse,
                api.get("/api/households") as HouseholdsResponse
            )
            guard state.owns(owner) else { return }
            state.household = data.household
            state.members = data.members
            state.historicalMembers = data.historicalMembers
            state.invites = data.invites
            state.userHouseholds = listData.households
            state.activeHouseholdId = data.household.id
        } catch {
            // Silent failure — PWA pattern
        }
    }

    func createHousehold(name: String, initials: String) async throws {
        let api = self.api.scoped()
        let body = CreateHouseholdRequest(name: name, initials: initials)
        let resp: HouseholdResponse = try await api.post("/api/household", body: body)
        guard api.identity.snapshot.user?.householdId == resp.household.id else { throw APIError.contextChanged }
        state.household = resp.household
        state.members = resp.members
        state.historicalMembers = resp.historicalMembers
        state.invites = resp.invites
        state.activeHouseholdId = resp.household.id
    }

    func joinHousehold(inviteCode: String) async throws {
        let api = self.api.scoped()
        let body = JoinHouseholdRequest(inviteCode: inviteCode)
        let resp: HouseholdResponse = try await api.post("/api/household/join", body: body)
        guard api.identity.snapshot.user?.householdId == resp.household.id else { throw APIError.contextChanged }
        state.household = resp.household
        state.members = resp.members
        state.historicalMembers = resp.historicalMembers
        state.invites = resp.invites
        state.activeHouseholdId = resp.household.id
    }
}
