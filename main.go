package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"time"

	"github.com/danopstech/octopusenergy"
	"github.com/wcharczuk/go-chart/v2"
)

/*
Plot octpus usage

FIXTHIS: generate email
FIXTHIS: support longer periods ( cleaner plots )

*/
func main() {

	today := time.Now()
	yesterday := time.Now().AddDate(0, 0, -1)
	defaultFrom := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, yesterday.Location())
	defaultTo := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())

	// Parse arguments
	//
	apiKey := flag.String("apikey", "", "API key from https://octopus.energy/dashboard/developer/")
	products := flag.Bool("listproducts", false, "List octopus products")
	dayReport := flag.Bool("dayreport", false, "Generate day report")
	mpan := flag.String("mpan", "", "Electricity meter point's MPAN from https://octopus.energy/dashboard/developer/")
	mprn := flag.String("mprn", "", "Gas meter point's MPRN from https://octopus.energy/dashboard/developer/")
	electricitySerial := flag.String("electricityserial", "", "Electricity meter's serial number from https://octopus.energy/dashboard/developer/")
	gasSerial := flag.String("gasserial", "", "Gas meter's serial number from https://octopus.energy/dashboard/developer/")
	electricityProductCode := flag.String("electricityproductcode", "", "Electricity product code")
	gasProductCode := flag.String("gasproductcode", "", "Gas product code")
	from := flag.String("from", defaultFrom.Format("2006-01-02T15:04:05Z"), "Report start date/time")
	to := flag.String("to", defaultTo.Format("2006-01-02T15:04:05Z"), "Report end date/time")
	signalUser := flag.String("signaluser", "", "Signal messenger username")
	signalRecipient := flag.String("signalrecipient", "", "Signal messenger recipient")

	flag.Parse()

	if len(*apiKey) == 0 {
		log.Println("apikey must be provided")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if *products {
		listProducts(apiKey)
		os.Exit(0)
	}

	if *dayReport {
		fromTime, err := time.Parse(time.RFC3339, *from)
		if err != nil {
			log.Fatalf("failed to parse from: %s", err.Error())
		}
		toTime, err := time.Parse(time.RFC3339, *to)
		if err != nil {
			log.Fatalf("failed to parse to: %s", err.Error())
		}
		if len(*mpan) == 0 {
			log.Println("mpan must be provided")
			flag.PrintDefaults()
			os.Exit(1)
		}
		if len(*mprn) == 0 {
			log.Println("mprn must be provided")
			flag.PrintDefaults()
			os.Exit(1)
		}
		if len(*electricitySerial) == 0 {
			log.Println("electricitySerial must be provided")
			flag.PrintDefaults()
			os.Exit(1)
		}
		if len(*gasSerial) == 0 {
			log.Println("gasSerial must be provided")
			flag.PrintDefaults()
			os.Exit(1)
		}
		if len(*electricityProductCode) == 0 {
			log.Println("electricityproductcode must be provided")
			flag.PrintDefaults()
			os.Exit(1)
		}
		if len(*gasProductCode) == 0 {
			log.Println("gasproductcode must be provided")
			flag.PrintDefaults()
			os.Exit(1)
		}

		electricityText, electricyImage, err := electricityReport(apiKey, mpan, electricitySerial, electricityProductCode, &fromTime, &toTime)
		if err != nil {
			log.Fatalf("Electricity failed: %s", err.Error())
		}
		if len(*signalUser) > 0 && len(*signalRecipient) > 0 {
			cmd := exec.Command("signal-cli", "-u", *signalUser, "send", *signalRecipient, "-m", electricityText, "-a", electricyImage)
			stdout, err := cmd.Output()
			if err != nil {
				log.Println(err.Error())
			}
			log.Println(string(stdout))
		}
		log.Println(electricityText + electricyImage)
		os.Remove(electricyImage)

		gasText, gasImage, err := gasReport(apiKey, mprn, gasSerial, gasProductCode, &fromTime, &toTime)
		if err != nil {
			log.Fatalf("Gas failed: %s", err.Error())
		}
		if len(*signalUser) > 0 && len(*signalRecipient) > 0 {
			cmd := exec.Command("signal-cli", "-u", *signalUser, "send", *signalRecipient, "-m", gasText, "-a", gasImage)
			stdout, err := cmd.Output()
			if err != nil {
				log.Println(err.Error())
			}
			log.Println(string(stdout))
		}
		os.Remove(gasImage)

	}
}

func listProducts(apiKey *string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	var netClient = http.Client{
		Timeout: time.Second * 10,
	}

	client := octopusenergy.NewClient(octopusenergy.NewConfig().
		WithApiKey(*apiKey).
		WithHTTPClient(netClient),
	)

	// List products
	//
	// Query for last 12 months to get recent tariffs
	//
	recentProducts := make(map[string]string)
	for i := 0; i > -12; i-- {
		products, err := client.Product.ListWithContext(ctx, &octopusenergy.ProductsListOptions{
			AvailableAt: octopusenergy.Time(time.Now().AddDate(0, i, 0)),
		})
		if err != nil {
			log.Fatalf("failed to list products: %s", err.Error())
		}
		for _, product := range products.Results {
			recentProducts[product.Code] = product.FullName
		}
	}
	var keys []string
	for k := range recentProducts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		log.Println("Code:", k, "FullName:", recentProducts[k])
	}
}

// FIX THIS: I expect gas & electricity can be merged
func electricityReport(apiKey *string, mpan *string, serialno *string, productCode *string, from *time.Time, to *time.Time) (string, string, error) {

	text := "Electricity: "

	tariffCode := "E-1R-" + *productCode + "-H"

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	var netClient = http.Client{
		Timeout: time.Second * 10,
	}

	client := octopusenergy.NewClient(octopusenergy.NewConfig().
		WithApiKey(*apiKey).
		WithHTTPClient(netClient),
	)

	// Get tariff charges
	//
	// just need 1 day - save the time ranges for later
	//
	tariffCharge, err := client.TariffCharge.GetWithContext(ctx, &octopusenergy.TariffChargesGetOptions{
		ProductCode: *productCode,
		TariffCode:  tariffCode,
		FuelType:    octopusenergy.FuelTypeElectricity,
		Rate:        octopusenergy.RateStandardUnit,
		PeriodFrom:  octopusenergy.Time(time.Now().Add(-24 * time.Hour)),
		PeriodTo:    octopusenergy.Time(time.Now()),
	})
	if err != nil {
		return "", "", errors.New("Failed to getting tariffCharge: " + err.Error())
	}

	// Get consumption
	//
	consumption, err := client.Consumption.GetPagesWithContext(ctx, &octopusenergy.ConsumptionGetOptions{
		MPN:          *mpan,
		SerialNumber: *serialno,
		FuelType:     octopusenergy.FuelTypeElectricity,
		PeriodFrom:   octopusenergy.Time(*from),
		PeriodTo:     octopusenergy.Time(*to),
	})
	if err != nil {
		return "", "", errors.New("failed getting consumption: " + err.Error())
	}
	var xaxis []time.Time
	var yaxisConsumption []float64
	var yaxisCost []float64
	totalCost := 0.0
	totalConsumption := 0.0
	// FIXTHIS:  messy ... avoid fixed length
	var totalConsumptionType [2]float64
	for _, c := range consumption.Results {
		consumptionStart, err := time.Parse(time.RFC3339, c.IntervalStart)
		xaxis = append(xaxis, consumptionStart)
		if err != nil {
			return "", "", errors.New("failed to parse consumption start: " + err.Error())
		}
		consumptionStartMinutes := consumptionStart.Local().Hour()*60 + consumptionStart.Local().Minute()
		rate := 0.0
		for i, t := range tariffCharge.Results {
			tariffFromMinutes := t.ValidFrom.Local().Hour()*60 + t.ValidFrom.Local().Minute()
			tariffToMinutes := t.ValidTo.Local().Hour()*60 + t.ValidTo.Local().Minute()
			if tariffToMinutes > tariffFromMinutes {
				if consumptionStartMinutes >= tariffFromMinutes && consumptionStartMinutes < tariffToMinutes {
					rate = t.ValueIncVat
					totalConsumptionType[i] = totalConsumptionType[i] + c.Consumption
					break
				}
			} else {
				if consumptionStartMinutes >= tariffFromMinutes || consumptionStartMinutes < tariffToMinutes {
					rate = t.ValueIncVat
					totalConsumptionType[i] = totalConsumptionType[i] + c.Consumption
					break
				}
			}
		}
		//log.Println("At: ", c.IntervalStart, " Hour: ", consumptionStart.Local().Hour(), " Consumption: ", c.Consumption, " Rate: ", rate, " Cost:", c.Consumption*rate)
		yaxisConsumption = append(yaxisConsumption, c.Consumption)
		yaxisCost = append(yaxisCost, c.Consumption*rate)
		totalCost = totalCost + c.Consumption*rate
		totalConsumption = totalConsumption + c.Consumption
	}

	//log.Println("yaxisCost:", yaxisCost)
	//log.Println("yaxisConsumption:", yaxisConsumption)
	//log.Println("xaxis:", xaxis)

	text = text + fmt.Sprintf("%.1fkWh (%.1f%%) %02d:%02d to %02d:%02d at %.1fp/kWh \n", totalConsumptionType[0],
		100.0*totalConsumptionType[0]/totalConsumption,
		tariffCharge.Results[0].ValidFrom.Hour(), tariffCharge.Results[0].ValidFrom.Minute(),
		tariffCharge.Results[0].ValidTo.Hour(), tariffCharge.Results[0].ValidTo.Minute(),
		tariffCharge.Results[0].ValueIncVat)
	text = text + fmt.Sprintf("%.1fkWh (%.1f%%) %02d:%02d to %02d:%02d at %.1fp/kWh \n", totalConsumptionType[1],
		100.0*totalConsumptionType[1]/totalConsumption,
		tariffCharge.Results[1].ValidFrom.Hour(), tariffCharge.Results[1].ValidFrom.Minute(),
		tariffCharge.Results[1].ValidTo.Hour(), tariffCharge.Results[1].ValidTo.Minute(),
		tariffCharge.Results[1].ValueIncVat)
	text = text + fmt.Sprintf("Total £%.2f for %.1fkWh, average %.1fp/kWh (inc VAT)\n", totalCost/100, totalConsumption, totalCost/totalConsumption)

	// chart
	//
	var ticks []chart.Tick
	for _, t := range xaxis {
		ticks = append(ticks, chart.Tick{Value: float64(t.UnixNano()), Label: t.Format("Jan-02-06 15:04")})
	}

	graph := chart.Chart{
		Title:      "Electricity",
		Background: chart.Style{Padding: chart.Box{Top: 20, Left: 20, Right: 20, Bottom: 20}},
		XAxis: chart.XAxis{
			Style: chart.Style{TextRotationDegrees: 90.0},
			Ticks: ticks,
		},
		YAxisSecondary: chart.YAxis{
			Name:      "Consumption kWh (1/2 hour)",
			NameStyle: chart.Style{FontColor: chart.ColorBlue},
		},
		YAxis: chart.YAxis{
			Name:      "Cost p",
			NameStyle: chart.Style{FontColor: chart.ColorRed},
		},
		Series: []chart.Series{
			chart.TimeSeries{
				YAxis:   chart.YAxisSecondary,
				XValues: xaxis,
				YValues: yaxisConsumption,
				Style:   chart.Style{StrokeColor: chart.ColorBlue, DotWidth: 3, DotColor: chart.ColorBlue},
			},
			chart.TimeSeries{
				YAxis:   chart.YAxisPrimary,
				XValues: xaxis,
				YValues: yaxisCost,
				Style:   chart.Style{StrokeColor: chart.ColorRed, DotWidth: 3, DotColor: chart.ColorRed},
			},
		},
	}

	f, _ := os.CreateTemp("", "*.png")
	defer f.Close()
	renderError := graph.Render(chart.PNG, f)
	if renderError != nil {
		return "", "", errors.New("failed render chart: " + renderError.Error())
	}

	return text, f.Name(), nil
}

func gasReport(apiKey *string, mprn *string, serialno *string, productCode *string, from *time.Time, to *time.Time) (string, string, error) {

	text := "Gas\n"

	tariffCode := "G-1R-" + *productCode + "-H"

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	var netClient = http.Client{
		Timeout: time.Second * 10,
	}

	client := octopusenergy.NewClient(octopusenergy.NewConfig().
		WithApiKey(*apiKey).
		WithHTTPClient(netClient),
	)

	// Get tariff charges
	//
	// just need 1 day - save the time ranges for later
	//
	tariffCharge, err := client.TariffCharge.GetWithContext(ctx, &octopusenergy.TariffChargesGetOptions{
		ProductCode: *productCode,
		TariffCode:  tariffCode,
		FuelType:    octopusenergy.FuelTypeGas,
		Rate:        octopusenergy.RateStandardUnit,
		PeriodFrom:  octopusenergy.Time(time.Now().Add(-24 * time.Hour)),
		PeriodTo:    octopusenergy.Time(time.Now()),
	})
	if err != nil {
		return "", "", errors.New("Failed to getting tariffCharge: " + err.Error())
	}

	// Get consumption
	//
	consumption, err := client.Consumption.GetPagesWithContext(ctx, &octopusenergy.ConsumptionGetOptions{
		MPN:          *mprn,
		SerialNumber: *serialno,
		FuelType:     octopusenergy.FuelTypeGas,
		PeriodFrom:   octopusenergy.Time(*from),
		PeriodTo:     octopusenergy.Time(*to),
	})
	if err != nil {
		return "", "", errors.New("failed getting consumption: " + err.Error())
	}
	if len(consumption.Results) == 0 {
		return "", "", errors.New("no consumption available")
	}

	var xaxis []time.Time
	var yaxisConsumption []float64
	var yaxisCost []float64
	totalCost := 0.0
	totalConsumption := 0.0
	for _, c := range consumption.Results {
		consumptionStart, err := time.Parse(time.RFC3339, c.IntervalStart)
		xaxis = append(xaxis, consumptionStart)
		if err != nil {
			return "", "", errors.New("failed to parse consumption start: " + err.Error())
		}
		consumptionStartMinutes := consumptionStart.Hour()*60 + consumptionStart.Minute()
		rate := 0.0
		for _, t := range tariffCharge.Results {
			tariffFromMinutes := t.ValidFrom.Hour()*60 + t.ValidFrom.Minute()
			tariffToMinutes := t.ValidTo.Hour()*60 + t.ValidTo.Minute()
			if tariffToMinutes > tariffFromMinutes {
				if consumptionStartMinutes >= tariffFromMinutes && consumptionStartMinutes < tariffToMinutes {
					rate = t.ValueIncVat
					break
				}
			} else {
				if consumptionStartMinutes >= tariffFromMinutes || consumptionStartMinutes < tariffToMinutes {
					rate = t.ValueIncVat
					break
				}
			}
		}
		yaxisConsumption = append(yaxisConsumption, c.Consumption)
		yaxisCost = append(yaxisCost, c.Consumption*rate)
		totalCost = totalCost + c.Consumption*rate
		totalConsumption = totalConsumption + c.Consumption
	}

	text = text + fmt.Sprintf("Total £%.2f for %.1fkWh (inc VAT)\n", totalCost/100, totalConsumption)

	// chart
	//
	var ticks []chart.Tick
	for _, t := range xaxis {
		ticks = append(ticks, chart.Tick{Value: float64(t.UnixNano()), Label: t.Format("Jan-02-06 15:04")})
	}

	graph := chart.Chart{
		Title:      "Gas",
		Background: chart.Style{Padding: chart.Box{Top: 20, Left: 20, Right: 20, Bottom: 20}},
		XAxis: chart.XAxis{
			Style: chart.Style{TextRotationDegrees: 90.0},
			Ticks: ticks,
		},
		YAxisSecondary: chart.YAxis{
			Name:      "Consumption kWh (1/2 hour)",
			NameStyle: chart.Style{FontColor: chart.ColorRed},
		},
		YAxis: chart.YAxis{
			Name:      "Cost p",
			NameStyle: chart.Style{FontColor: chart.ColorBlue},
		},
		Series: []chart.Series{
			chart.TimeSeries{
				YAxis:   chart.YAxisSecondary,
				XValues: xaxis,
				YValues: yaxisCost,
				Style:   chart.Style{StrokeColor: chart.ColorBlue, DotWidth: 3, DotColor: chart.ColorBlue},
			},
			chart.TimeSeries{
				XValues: xaxis,
				YValues: yaxisConsumption,
				Style:   chart.Style{StrokeColor: chart.ColorRed, DotWidth: 3, DotColor: chart.ColorRed},
			},
		},
	}

	f, _ := os.CreateTemp("", "*.png")
	defer f.Close()
	renderError := graph.Render(chart.PNG, f)
	if renderError != nil {
		return "", "", errors.New("failed render chart: " + renderError.Error())
	}

	return text, f.Name(), nil
}
