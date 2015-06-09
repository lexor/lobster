package lobster

import "net/http"
import "github.com/gorilla/mux"
import "fmt"
import "strconv"
import "strings"
import "time"

type FrameParams struct {
	Message string
	Error bool
	Scripts []string
}
type PanelFormParams struct {
	Frame FrameParams
	Token string
}

type PanelHandlerFunc func(http.ResponseWriter, *http.Request, *Database, *Session, FrameParams)

func panelWrap(h PanelHandlerFunc) func(http.ResponseWriter, *http.Request, *Database, *Session) {
	return func(w http.ResponseWriter, r *http.Request, db *Database, session *Session) {
		if !session.IsLoggedIn() {
			http.Redirect(w, r, "/login", 303)
		} else {
			var frameParams FrameParams
			if r.URL.Query()["message"] != nil {
				frameParams.Message = r.URL.Query()["message"][0]
				if strings.HasPrefix(frameParams.Message, "Error") {
					frameParams.Error = true
				}
			}
			h(w, r, db, session, frameParams)
		}
	}
}

type PanelDashboardParams struct {
	Frame FrameParams
	VirtualMachines []*VirtualMachine
	CreditSummary *CreditSummary
	BandwidthSummary map[string]*BandwidthSummary
	Tickets []*Ticket
}
func panelDashboard(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	params := PanelDashboardParams{}
	params.Frame = frameParams
	params.VirtualMachines = vmList(db, session.UserId)
	params.CreditSummary = userCreditSummary(db, session.UserId)
	params.BandwidthSummary = userBandwidthSummary(db, session.UserId)
	params.Tickets = ticketListActive(db, session.UserId)
	renderTemplate(w, "panel", "dashboard", params)
}

type PanelVirtualMachinesParams struct {
	Frame FrameParams
	VirtualMachines []*VirtualMachine
}
func panelVirtualMachines(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	params := PanelVirtualMachinesParams{}
	params.Frame = frameParams
	params.VirtualMachines = vmList(db, session.UserId)
	renderTemplate(w, "panel", "vms", params)
}

type PanelNewVMParams struct {
	Frame FrameParams
	Regions []string
}
func panelNewVM(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	params := PanelNewVMParams{}
	params.Frame = frameParams
	params.Regions = regionList()
	renderTemplate(w, "panel", "newvm", params)
}

type PanelNewVMRegionParams struct {
	Frame FrameParams
	Region string
	PublicImages []*Image
	UserImages []*Image
	Plans []*Plan
	Token string
}
type NewVMRegionForm struct {
	Name string `schema:"name"`
	PlanId int `schema:"plan_id"`
	ImageId int `schema:"image_id"`
}
func panelNewVMRegion(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	region := mux.Vars(r)["region"]

	if r.Method == "POST" {
		form := new(NewVMRegionForm)
		err := decoder.Decode(form, r.PostForm)
		if err != nil {
			http.Redirect(w, r, "/panel/newvm/" + region, 303)
			return
		}

		vmId, err := vmCreate(db, session.UserId, form.Name, form.PlanId, form.ImageId)
		if err != nil {
			redirectMessage(w, r, "/panel/newvm/" + region, "Error: " + err.Error() + ".")
		} else {
			LogAction(db, session.UserId, extractIP(r.RemoteAddr), "Create VM", fmt.Sprintf("Name: %s, Plan: %d, Image: %d", form.Name, form.PlanId, form.ImageId))
			http.Redirect(w, r, fmt.Sprintf("/panel/vm/%d", vmId), 303)
		}
		return
	}

	params := PanelNewVMRegionParams{}
	params.Frame = frameParams
	params.Region = region
	params.Plans = planList(db)
	params.Token = csrfGenerate(db, session)

	for _, image := range imageListRegion(db, session.UserId, region) {
		if image.UserId == -1 {
			params.PublicImages = append(params.PublicImages, image)
		} else {
			params.UserImages = append(params.UserImages, image)
		}
	}

	renderTemplate(w, "panel", "newvm_region", params)
}

type PanelVMParams struct {
	Frame FrameParams
	Vm *VirtualMachine
	Images []*Image
	Token string
}
func panelVM(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	vmId, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 32)
	if err != nil {
		http.Redirect(w, r, "/panel/vms", 303)
		return
	}
	vm := vmInfo(db, session.UserId, int(vmId))
	if vm == nil {
		http.Redirect(w, r, "/panel/vms", 303)
		return
	}

	params := PanelVMParams{}
	params.Frame = frameParams
	params.Vm = vm
	params.Images = imageListRegion(db, session.UserId, vm.Region)
	params.Token = csrfGenerate(db, session)
	renderTemplate(w, "panel", "vm", params)
}

// virtual machine actions
func panelVMStart(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	vmId, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 32)
	if err != nil {
		http.Redirect(w, r, "/panel/vms", 303)
		return
	}
	err = vmStart(db, session.UserId, int(vmId))
	if err != nil {
		redirectMessage(w, r, fmt.Sprintf("/panel/vm/%d", vmId), "Error: " + err.Error() + ".")
	} else {
		LogAction(db, session.UserId, extractIP(r.RemoteAddr), "Start VM", fmt.Sprintf("VM ID: %d", vmId))
		redirectMessage(w, r, fmt.Sprintf("/panel/vm/%d", vmId), "VM started successfully.")
	}
}
func panelVMStop(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	vmId, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 32)
	if err != nil {
		http.Redirect(w, r, "/panel/vms", 303)
		return
	}
	err = vmStop(db, session.UserId, int(vmId))
	if err != nil {
		redirectMessage(w, r, fmt.Sprintf("/panel/vm/%d", vmId), "Error: " + err.Error() + ".")
	} else {
		LogAction(db, session.UserId, extractIP(r.RemoteAddr), "Stop VM", fmt.Sprintf("VM ID: %d", vmId))
		redirectMessage(w, r, fmt.Sprintf("/panel/vm/%d", vmId), "VM stopped successfully.")
	}
}
func panelVMReboot(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	vmId, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 32)
	if err != nil {
		http.Redirect(w, r, "/panel/vms", 303)
		return
	}
	err = vmReboot(db, session.UserId, int(vmId))
	if err != nil {
		redirectMessage(w, r, fmt.Sprintf("/panel/vm/%d", vmId), "Error: " + err.Error() + ".")
	} else {
		LogAction(db, session.UserId, extractIP(r.RemoteAddr), "Reboot VM", fmt.Sprintf("VM ID: %d", vmId))
		redirectMessage(w, r, fmt.Sprintf("/panel/vm/%d", vmId), "VM rebooted successfully.")
	}
}
func panelVMDelete(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	vmId, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 32)
	if err != nil {
		http.Redirect(w, r, "/panel/vms", 303)
		return
	}
	err = vmDelete(db, session.UserId, int(vmId))
	if err != nil {
		redirectMessage(w, r, fmt.Sprintf("/panel/vm/%d", vmId), "Error: " + err.Error() + ".")
	} else {
		LogAction(db, session.UserId, extractIP(r.RemoteAddr), "Delete VM", fmt.Sprintf("VM ID: %d", vmId))
		redirectMessage(w, r, "/panel/vms", "VM deleted successfully.")
	}
}
func panelVMAction(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	vmId, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 32)
	action := mux.Vars(r)["action"]
	if err != nil {
		http.Redirect(w, r, "/panel/vms", 303)
		return
	}

	err = vmAction(db, session.UserId, int(vmId), action, r.PostFormValue("value"))
	if err != nil {
		redirectMessage(w, r, fmt.Sprintf("/panel/vm/%d", vmId), "Error: " + err.Error() + ".")
	} else {
		LogAction(db, session.UserId, extractIP(r.RemoteAddr), "VM action", fmt.Sprintf("VM ID: %d; Action: %s", vmId, action))
		redirectMessage(w, r, fmt.Sprintf("/panel/vm/%d", vmId), "Action [" + action + "] applied successfully.")
	}
}

func panelVMVnc(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	vmId, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 32)
	if err != nil {
		http.Redirect(w, r, "/panel/vms", 303)
		return
	}
	url, err := vmVnc(db, session.UserId, int(vmId))
	if err != nil {
		redirectMessage(w, r, fmt.Sprintf("/panel/vm/%d", vmId), "Error: " + err.Error() + ".")
	} else {
		renderTemplate(w, "panel", "vnc", url)
	}
}

type VMReimageForm struct {
	Image int `schema:"image"`
}
func panelVMReimage(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	vmId, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 32)
	if err != nil {
		http.Redirect(w, r, "/panel/vms", 303)
		return
	}

	form := new(VMReimageForm)
	err = decoder.Decode(form, r.PostForm)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/panel/vm/%d", vmId), 303)
		return
	}

	err = vmReimage(db, session.UserId, int(vmId), form.Image)
	if err != nil {
		redirectMessage(w, r, fmt.Sprintf("/panel/vm/%d", vmId), "Error: " + err.Error() + ".")
	} else {
		LogAction(db, session.UserId, extractIP(r.RemoteAddr), "Re-image VM", fmt.Sprintf("VM ID: %d; Image: %d", vmId, form.Image))
		redirectMessage(w, r, fmt.Sprintf("/panel/vm/%d", vmId), "VM re-image in progress.")
	}
}

func panelVMRename(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	vmId, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 32)
	if err != nil {
		http.Redirect(w, r, "/panel/vms", 303)
		return
	}

	err = vmRename(db, session.UserId, int(vmId), r.PostFormValue("name"))
	if err != nil {
		redirectMessage(w, r, fmt.Sprintf("/panel/vm/%d", vmId), "Error: " + err.Error() + ".")
	} else {
		LogAction(db, session.UserId, extractIP(r.RemoteAddr), "Rename VM", fmt.Sprintf("VM ID: %d; Name: %d", vmId, r.PostFormValue("name")))
		redirectMessage(w, r, fmt.Sprintf("/panel/vm/%d", vmId), "VM renamed successfully.")
	}
}

type PanelBillingParams struct {
	Frame FrameParams
	CreditSummary *CreditSummary
	PaymentMethods []string
}
func panelBilling(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	params := PanelBillingParams{}
	params.Frame = frameParams
	params.CreditSummary = userCreditSummary(db, session.UserId)
	params.PaymentMethods = paymentMethodList()
	renderTemplate(w, "panel", "billing", params)
}

type PayForm struct {
	Gateway string `schema:"gateway"`
	Amount float64 `schema:"amount"`
}
func panelPay(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	form := new(PayForm)
	err := decoder.Decode(form, r.Form)
	if err != nil {
		http.Redirect(w, r, "/panel/billing", 303)
		return
	}

	if form.Amount < 5 || form.Amount > 300 {
		redirectMessage(w, r, "/panel/billing", "Error: amount must be between $5.00 and $300.00.")
		return
	}

	user := userDetails(db, session.UserId)
	paymentHandle(form.Gateway, w, r, frameParams, session.UserId, user.Username, form.Amount)
}

type PanelChargesParams struct {
	Frame FrameParams
	Year int
	Month time.Month
	Charges []*Charge

	Previous time.Time
	Next time.Time
}
func panelCharges(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	year, err := strconv.ParseInt(mux.Vars(r)["year"], 10, 32)
	if err != nil {
		year = int64(time.Now().Year())
	}
	month, err := strconv.ParseInt(mux.Vars(r)["month"], 10, 32)
	if err != nil {
		month = int64(time.Now().Month())
	}

	requestTime := time.Date(int(year), time.Month(month), 1, 0, 0, 0, 0, time.UTC)

	params := PanelChargesParams{}
	params.Frame = frameParams
	params.Year = int(year)
	params.Month = time.Month(month)
	params.Charges = chargeList(db, session.UserId, params.Year, params.Month)
	params.Previous = requestTime.AddDate(0, -1, 0)
	params.Next = requestTime.AddDate(0, 1, 0)
	renderTemplate(w, "panel", "charges", params)
}

type PanelAccountParams struct {
	Frame FrameParams
	User *User
	Token string
}
func panelAccount(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	params := PanelAccountParams{}
	params.Frame = frameParams
	params.User = userDetails(db, session.UserId)
	params.Token = csrfGenerate(db, session)
	renderTemplate(w, "panel", "account", params)
}

type AccountPasswordForm struct {
	OldPassword string `schema:"old_password"`
	NewPassword string `schema:"new_password"`
	NewPasswordConfirm string `schema:"new_password_confirm"`
}
func panelAccountPassword(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	form := new(AccountPasswordForm)
	err := decoder.Decode(form, r.PostForm)
	if err != nil {
		http.Redirect(w, r, "/panel/account", 303)
		return
	} else if form.NewPassword != form.NewPasswordConfirm {
		redirectMessage(w, r, "/panel/account", "Error: new passwords do not match.")
	}

	err = authChangePassword(db, extractIP(r.RemoteAddr), session.UserId, form.OldPassword, form.NewPassword)
	if err != nil {
		redirectMessage(w, r, "/panel/account", "Error: " + err.Error() + ".")
	} else {
		redirectMessage(w, r, "/panel/account", "Password changed successfully.")
	}
}

type PanelImagesParams struct {
	Frame FrameParams
	Images []*Image
	Regions []string
	Token string
}
func panelImages(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	params := PanelImagesParams{}
	params.Frame = frameParams
	params.Regions = regionList()
	params.Token = csrfGenerate(db, session)

	for _, image := range imageList(db, session.UserId) {
		if image.UserId == session.UserId {
			params.Images = append(params.Images, image)
		}
	}

	renderTemplate(w, "panel", "images", params)
}

type ImageAddForm struct {
	Region string `schema:"region"`
	Name string `schema:"name"`
	Location string `schema:"location"`
	Format string `schema:"format"`
}
func panelImageAdd(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	form := new(ImageAddForm)
	err := decoder.Decode(form, r.PostForm)
	if err != nil {
		http.Redirect(w, r, "/panel/images", 303)
		return
	}

	_, err = imageFetch(db, session.UserId, form.Region, form.Name, form.Location, form.Format)
	if err != nil {
		redirectMessage(w, r, "/panel/images", "Error: " + err.Error() + ".")
	} else {
		LogAction(db, session.UserId, extractIP(r.RemoteAddr), "Add image", fmt.Sprintf("Location: %s; Format: %s; Name: %s", form.Location, form.Format, form.Name))
		redirectMessage(w, r, "/panel/images", "Image download is in progress (the image will be available once the status becomes active in the list below).")
	}
}

func panelImageRemove(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	imageId, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 32)
	if err != nil {
		http.Redirect(w, r, "/panel/images", 303)
		return
	}

	err = imageDelete(db, session.UserId, int(imageId))
	if err != nil {
		redirectMessage(w, r, "/panel/images", "Error: " + err.Error() + ".")
	} else {
		LogAction(db, session.UserId, extractIP(r.RemoteAddr), "Remove image", fmt.Sprintf("ID: %d", imageId))
		redirectMessage(w, r, "/panel/images", "Image removed.")
	}
}

type PanelImageDetailsParams struct {
	Frame FrameParams
	Image *Image
}
func panelImageDetails(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	imageId, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 32)
	if err != nil {
		http.Redirect(w, r, "/panel/images", 303)
		return
	}
	image := imageInfo(db, session.UserId, int(imageId))
	if image == nil {
		http.Redirect(w, r, "/panel/images", 303)
		return
	}

	params := PanelImageDetailsParams{}
	params.Frame = frameParams
	params.Image = image
	renderTemplate(w, "panel", "image_details", params)
}

type SupportParams struct {
	Frame FrameParams
	Tickets []*Ticket
}
func panelSupport(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	params := SupportParams{}
	params.Frame = frameParams
	params.Tickets = ticketList(db, session.UserId)
	renderTemplate(w, "panel", "support", params)
}

type SupportOpenForm struct {
	Name string `schema:"name"`
	Message string `schema:"message"`
}
func panelSupportOpen(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	if r.Method == "POST" {
		form := new(SupportOpenForm)
		err := decoder.Decode(form, r.PostForm)
		if err != nil {
			http.Redirect(w, r, "/panel/support/open", 303)
			return
		}

		ticketId, err := ticketOpen(db, session.UserId, form.Name, form.Message, false)
		if err != nil {
			redirectMessage(w, r, "/panel/support/open", "Error: " + err.Error() + ".")
		} else {
			LogAction(db, session.UserId, extractIP(r.RemoteAddr), "Open ticket", fmt.Sprintf("Subject: %s; Ticket ID: %d", form.Name, ticketId))
			http.Redirect(w, r, fmt.Sprintf("/panel/support/%d", ticketId), 303)
		}
		return
	}

	renderTemplate(w, "panel", "support_open", PanelFormParams{Frame: frameParams, Token: csrfGenerate(db, session)})
}

type PanelSupportTicketParams struct {
	Frame FrameParams
	Ticket *Ticket
	Token string
}
func panelSupportTicket(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	ticketId, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 32)
	if err != nil {
		http.Redirect(w, r, "/panel/support", 303)
		return
	}
	ticket := ticketDetails(db, session.UserId, int(ticketId), false)
	if ticket == nil {
		http.Redirect(w, r, "/panel/support", 303)
		return
	}

	params := PanelSupportTicketParams{}
	params.Frame = frameParams
	params.Ticket = ticket
	params.Token = csrfGenerate(db, session)
	renderTemplate(w, "panel", "support_ticket", params)
}

type SupportTicketReplyForm struct {
	Message string `schema:"message"`
}
func panelSupportTicketReply(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	ticketId, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 32)
	if err != nil {
		http.Redirect(w, r, "/panel/support", 303)
		return
	}
	form := new(SupportTicketReplyForm)
	err = decoder.Decode(form, r.PostForm)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/panel/support/%d", ticketId), 303)
		return
	}

	err = ticketReply(db, session.UserId, int(ticketId), form.Message, false)
	if err != nil {
		redirectMessage(w, r, fmt.Sprintf("/panel/support/%d", ticketId), "Error: " + err.Error() + ".")
	} else {
		LogAction(db, session.UserId, extractIP(r.RemoteAddr), "Ticket reply", fmt.Sprintf("Ticket ID: %d", ticketId))
		http.Redirect(w, r, fmt.Sprintf("/panel/support/%d", ticketId), 303)
	}
}

func panelSupportTicketClose(w http.ResponseWriter, r *http.Request, db *Database, session *Session, frameParams FrameParams) {
	ticketId, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 32)
	if err != nil {
		http.Redirect(w, r, "/panel/support", 303)
		return
	}
	ticketClose(db, session.UserId, int(ticketId))
	LogAction(db, session.UserId, extractIP(r.RemoteAddr), "Close ticket", fmt.Sprintf("Ticket ID: %d", ticketId))
	redirectMessage(w, r, fmt.Sprintf("/panel/support/%d", ticketId), "This ticket has been marked closed.")
}