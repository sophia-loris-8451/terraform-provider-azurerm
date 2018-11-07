package azurerm

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform/helper/acctest"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/azure"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/utils"
)

func TestAccAzureRMRecoveryServicesProtectedVm_basic(t *testing.T) {
	resourceName := "azurerm_recovery_services_protected_vm.test"
	ri := acctest.RandInt()

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testCheckAzureRMRecoveryServicesProtectedVmDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccAzureRMRecoveryServicesProtectedVm_basic(ri, testLocation()),
				Check: resource.ComposeTestCheckFunc(
					testCheckAzureRMRecoveryServicesProtectedVmExists(resourceName),
					resource.TestCheckResourceAttrSet(resourceName, "resource_group_name"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{ //vault cannot be deleted unless we unregister all backups
				Config: testAccAzureRMRecoveryServicesProtectedVm_base(ri, testLocation()),
				Check:  resource.ComposeTestCheckFunc(),
			},
		},
	})
}

func testCheckAzureRMRecoveryServicesProtectedVmDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "azurerm_recovery_services_protected_vm" {
			continue
		}

		resourceGroup := rs.Primary.Attributes["resource_group_name"]
		vaultName := rs.Primary.Attributes["recovery_vault_name"]
		vmName := rs.Primary.Attributes["source_vm_name"]

		protectedItemName := fmt.Sprintf("VM;iaasvmcontainerv2;%s;%s", resourceGroup, vmName)
		containerName := fmt.Sprintf("iaasvmcontainer;iaasvmcontainerv2;%s;%s", resourceGroup, vmName)

		client := testAccProvider.Meta().(*ArmClient).recoveryServicesProtectedItemsClient
		ctx := testAccProvider.Meta().(*ArmClient).StopContext

		resp, err := client.Get(ctx, vaultName, resourceGroup, "Azure", containerName, protectedItemName, "")
		if err != nil {
			if utils.ResponseWasNotFound(resp.Response) {
				return nil
			}

			return err
		}

		return fmt.Errorf("Recovery Services Protected VM still exists:\n%#v", resp)
	}

	return nil
}

func testCheckAzureRMRecoveryServicesProtectedVmExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		// Ensure we have enough information in state to look up in API
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("Not found: %q", resourceName)
		}

		resourceGroup, hasResourceGroup := rs.Primary.Attributes["resource_group_name"]
		if !hasResourceGroup {
			return fmt.Errorf("Bad: no resource group found in state for Recovery Services Protected VM: %q", resourceName)
		}

		vaultName := rs.Primary.Attributes["recovery_vault_name"]
		vmId := rs.Primary.Attributes["source_vm_id"]

		//get VM name from id
		parsedVmId, err := azure.ParseAzureResourceID(vmId)
		if err != nil {
			return fmt.Errorf("[ERROR] Unable to parse source_vm_id '%s': %+v", vmId, err)
		}
		vmName, hasName := parsedVmId.Path["virtualMachines"]
		if !hasName {
			return fmt.Errorf("[ERROR] parsed source_vm_id '%s' doesn't contain 'virtualMachines'", vmId)
		}

		protectedItemName := fmt.Sprintf("VM;iaasvmcontainerv2;%s;%s", resourceGroup, vmName)
		containerName := fmt.Sprintf("iaasvmcontainer;iaasvmcontainerv2;%s;%s", resourceGroup, vmName)

		client := testAccProvider.Meta().(*ArmClient).recoveryServicesProtectedItemsClient
		ctx := testAccProvider.Meta().(*ArmClient).StopContext

		resp, err := client.Get(ctx, vaultName, resourceGroup, "Azure", containerName, protectedItemName, "")
		if err != nil {
			if utils.ResponseWasNotFound(resp.Response) {
				return fmt.Errorf("Recovery Services Protected VM %q (resource group: %q) was not found: %+v", protectedItemName, resourceGroup, err)
			}

			return fmt.Errorf("Bad: Get on recoveryServicesVaultsClient: %+v", err)
		}

		return nil
	}
}

func testAccAzureRMRecoveryServicesProtectedVm_base(rInt int, location string) string {
	return fmt.Sprintf(` 
resource "azurerm_resource_group" "test" {
  name     = "acctestRG-%[1]d"
  location = "%[2]s"
}

resource "azurerm_virtual_network" "test" {
  name                = "vnet"
  location            = "${azurerm_resource_group.test.location}"
  address_space       = ["10.0.0.0/16"]
  resource_group_name = "${azurerm_resource_group.test.name}"
}

resource "azurerm_subnet" "test" {
  name                 = "acctest_subnet"
  virtual_network_name = "${azurerm_virtual_network.test.name}"
  resource_group_name  = "${azurerm_resource_group.test.name}"
  address_prefix       = "10.0.10.0/24"
}

resource "azurerm_network_interface" "test" {
  name                = "acctest_nic"
  location            = "${azurerm_resource_group.test.location}"
  resource_group_name = "${azurerm_resource_group.test.name}"

  ip_configuration {
    name                          = "acctestipconfig"
    subnet_id                     = "${azurerm_subnet.test.id}"
    private_ip_address_allocation = "Dynamic"
    public_ip_address_id          = "${azurerm_public_ip.test.id}"
  }
}

resource "azurerm_public_ip" "test" {
  name                         = "acctest-ip"
  location                     = "${azurerm_resource_group.test.location}"
  resource_group_name          = "${azurerm_resource_group.test.name}"
  public_ip_address_allocation = "Dynamic"
  domain_name_label            = "acctestip%[1]d"
}

resource "azurerm_storage_account" "test" {
  name                     = "acctest%[3]s"
  location                 = "${azurerm_resource_group.test.location}"
  resource_group_name      = "${azurerm_resource_group.test.name}"
  account_tier             = "Standard"
  account_replication_type = "LRS"
}

resource "azurerm_managed_disk" "test" {
  name                 = "acctest-datadisk"
  location             = "${azurerm_resource_group.test.location}"
  resource_group_name  = "${azurerm_resource_group.test.name}"
  storage_account_type = "Standard_LRS"
  create_option        = "Empty"
  disk_size_gb         = "1023"
}

resource "azurerm_virtual_machine" "test" {
  name                  = "acctestvm"
  location              = "${azurerm_resource_group.test.location}"
  resource_group_name   = "${azurerm_resource_group.test.name}"
  vm_size               = "Standard_A0"
  network_interface_ids = ["${azurerm_network_interface.test.id}"]

  storage_image_reference {
    publisher = "Canonical"
    offer     = "UbuntuServer"
    sku       = "16.04-LTS"
    version   = "latest"
  }

  storage_os_disk {
    name              = "acctest-osdisk"
    managed_disk_type = "Standard_LRS"
    caching           = "ReadWrite"
    create_option     = "FromImage"
  }

  storage_data_disk {
    name              = "acctest-datadisk"
    managed_disk_id   = "${azurerm_managed_disk.test.id}"
    managed_disk_type = "Standard_LRS"
    disk_size_gb      = "${azurerm_managed_disk.test.disk_size_gb}"
    create_option     = "Attach"
    lun               = 0
  }

  os_profile {
    computer_name  = "acctest"
    admin_username = "vmadmin"
    admin_password = "Password123!@#"
  }

  os_profile_linux_config {
    disable_password_authentication = false
  }

  boot_diagnostics {
    enabled     = true
    storage_uri = "${azurerm_storage_account.test.primary_blob_endpoint}"
  }
}

resource "azurerm_recovery_services_vault" "test" {
  name                = "acctest-%[1]d"
  location            = "${azurerm_resource_group.test.location}"
  resource_group_name = "${azurerm_resource_group.test.name}"
  sku                 = "Standard"
}

resource "azurerm_recovery_services_protection_policy_vm" "test" {
  name                = "acctest-%[1]d"
  resource_group_name = "${azurerm_resource_group.test.name}"
  recovery_vault_name = "${azurerm_recovery_services_vault.test.name}"

  backup = {
    frequency = "Daily"
    time      = "23:00"
  }

  retention_daily = {
    count = 10
  }
}
`, rInt, location, strconv.Itoa(rInt)[0:5])
}

func testAccAzureRMRecoveryServicesProtectedVm_basic(rInt int, location string) string {
	return fmt.Sprintf(`
%s

resource "azurerm_recovery_services_protected_vm" "test" {
  resource_group_name = "${azurerm_resource_group.test.name}"
  recovery_vault_name = "${azurerm_recovery_services_vault.test.name}"
  source_vm_id        = "${azurerm_virtual_machine.test.id}"
  backup_policy_id    = "${azurerm_recovery_services_protection_policy_vm.test.id}"
}

`, testAccAzureRMRecoveryServicesProtectedVm_base(rInt, location))
}
