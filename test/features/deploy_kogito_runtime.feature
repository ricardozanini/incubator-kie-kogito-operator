@smoke
Feature: Deploy Kogito Runtime

  Background:
    Given Namespace is created

  Scenario: Deploy Kogito Runtime for a Spring Boot application
    Given Kogito Operator is deployed

    When Deploy springboot example runtime service "quay.io/jcarvaja/process-springboot-example:8.0.0" with configuration:
      | config | persistence | disabled |
    And Expose runtime service "process-springboot-example"
    
    Then Kogito application "process-springboot-example" has 1 pods running within 10 minutes
    And Service "process-springboot-example" with process name "orders" is available within 2 minutes
