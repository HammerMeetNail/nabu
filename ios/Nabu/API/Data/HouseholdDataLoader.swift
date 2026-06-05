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
        do {
            let (data, listData) = try await (
                api.get("/api/household") as HouseholdResponse,
                api.get("/api/households") as HouseholdsResponse
            )
            state.household = data.household
            state.members = data.members
            state.invites = data.invites
            state.userHouseholds = listData.households
            state.activeHouseholdId = data.household.id
        } catch {
            // Silent failure — PWA pattern
        }
    }

    func createHousehold(name: String, initials: String) async throws {
        let body = CreateHouseholdRequest(name: name, initials: initials)
        let resp: HouseholdResponse = try await api.post("/api/household", body: body)
        state.household = resp.household
        state.members = resp.members
        state.invites = resp.invites
        state.activeHouseholdId = resp.household.id
    }

    func joinHousehold(inviteCode: String) async throws {
        let body = JoinHouseholdRequest(inviteCode: inviteCode)
        let resp: HouseholdResponse = try await api.post("/api/household/join", body: body)
        state.household = resp.household
        state.members = resp.members
        state.invites = resp.invites
        state.activeHouseholdId = resp.household.id
    }
}
