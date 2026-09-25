// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Script, console2} from "forge-std/Script.sol";
import {AppraisalRegistry} from "../src/AppraisalRegistry.sol";
import {GradedCard} from "../src/GradedCard.sol";
import {TestUSD} from "../src/TestUSD.sol";
import {CardLendingVault} from "../src/CardLendingVault.sol";

/// v2: new GradedCard (adds claimable demo copies) + new vault bound to it.
/// Keeps the existing AppraisalRegistry (all certificates) and TestUSD.
///   forge script script/DeployV2.s.sol --rpc-url robinhood_testnet --broadcast
contract DeployV2 is Script {
    function run() external {
        uint256 pk = vm.envUint("PRIVATE_KEY");
        address deployer = vm.addr(pk);
        string memory v1 = vm.readFile(string.concat("deployments/", vm.toString(block.chainid), ".json"));
        AppraisalRegistry registry = AppraisalRegistry(vm.parseJsonAddress(v1, ".registry"));
        TestUSD usd = TestUSD(vm.parseJsonAddress(v1, ".usd"));
        string memory seed = vm.readFile("data/seed_cards.json");
        uint256 n = vm.envOr("CARD_COUNT", uint256(6)); // seed_cards.json has 6 cards

        vm.startBroadcast(pk);
        GradedCard cards = new GradedCard(deployer);
        CardLendingVault vault = new CardLendingVault(deployer, registry, cards, usd);
        usd.mint(address(vault), 1_000_000e6);
        for (uint256 i = 0; i < n; i++) {
            string memory p = string.concat(".cards[", vm.toString(i), "]");
            string memory grader = abi.decode(vm.parseJson(seed, string.concat(p, ".grader")), (string));
            string memory cert = abi.decode(vm.parseJson(seed, string.concat(p, ".certId")), (string));
            string memory grade = abi.decode(vm.parseJson(seed, string.concat(p, ".grade")), (string));
            string memory name = abi.decode(vm.parseJson(seed, string.concat(p, ".name")), (string));
            cards.mint(deployer, grader, cert, grade, name);   // token i+1 = the real slab
            cards.addDemoCard(grader, cert, grade, name);      // demo index i
        }
        vm.stopBroadcast();

        console2.log("GradedCard v2       ", address(cards));
        console2.log("CardLendingVault v2 ", address(vault));
        string memory j = "v2";
        vm.serializeUint(j, "chainId", block.chainid);
        vm.serializeAddress(j, "registry", address(registry));
        vm.serializeAddress(j, "usd", address(usd));
        vm.serializeAddress(j, "cards", address(cards));
        vm.serializeAddress(j, "vault", address(vault));
        vm.serializeAddress(j, "cardsV1", vm.parseJsonAddress(v1, ".cards"));
        string memory out = vm.serializeAddress(j, "vaultV1", vm.parseJsonAddress(v1, ".vault"));
        vm.writeJson(out, string.concat("deployments/", vm.toString(block.chainid), "-v2.json"));
    }
}
