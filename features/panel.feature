# language: en
Feature: Hysteria 2 panel management
  As an administrator
  I want to manage users and the server config
  So that I can run Hysteria 2 without editing YAML by hand

  Background:
    Given a fresh panel with admin credentials

  Scenario: successful login returns a bearer token
    When I login with admin credentials
    Then the response code is 200
    And the response contains a token

  Scenario: login with wrong password is rejected
    When I login with a wrong password
    Then the response code is 401
    And the response has no token

  Scenario: API requires authentication
    When I request "/api/users" without a token
    Then the response code is 401

  Scenario: creating a user syncs the hysteria config
    Given I am authenticated
    When I create user "ivan" with password "pw1234"
    Then the response code is 201
    And the hysteria config contains user "ivan"
    And the hysteria service was restarted

  Scenario: duplicate user is rejected
    Given I am authenticated
    And user "lena" exists
    When I create user "lena" with password "other"
    Then the response code is 409

  Scenario: invalid username is rejected
    Given I am authenticated
    When I create user "bad name!" with password "pw1234"
    Then the response code is 400

  Scenario: disabling a user removes him from userpass map
    Given I am authenticated
    And user "maria" exists
    When I disable user "maria"
    Then the hysteria config has NO user "maria" in userpass

  Scenario: deleting a user removes him from the store and config
    Given I am authenticated
    And user "kolya" exists
    When I delete user "kolya"
    Then the response code is 200
    And the hysteria config has NO user "kolya" in userpass

  Scenario: changing listen port is validated
    Given I am authenticated
    When I set config port to 70000
    Then the response code is 400
    When I set config port to 2053
    Then the response code is 200
    And the hysteria config listen port is 2053

  Scenario: generated share link contains credentials
    Given I am authenticated
    And user "sveta" with password "pw1234" exists
    And panel domain is "vpn.example.com"
    When I request the share link for "sveta"
    Then the link starts with "hysteria2://sveta:pw1234@vpn.example.com"
    And the link contains SNI

  Scenario: unknown service action is rejected
    Given I am authenticated
    When I send service action "explode"
    Then the response code is 400
